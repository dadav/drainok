package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dadav/drainok/internal/analyzer"
	"github.com/dadav/drainok/internal/checks"
	"github.com/dadav/drainok/internal/kube"
	"github.com/dadav/drainok/internal/output"
)

// ErrNotDrainable signals exit code 1: the analysis ran fine but at least one
// evaluated node is not drainable.
var ErrNotDrainable = errors.New("one or more nodes are not drainable")

var configFile string

var rootCmd = &cobra.Command{
	Use:   "drainok [node...]",
	Short: "Check which Kubernetes nodes are drainable",
	Long: `drainok checks every node (or the named nodes) of a cluster against a set
of drainability conditions and reports which nodes could be drained right
now. It is read-only: nothing is cordoned or evicted.

Each node is evaluated independently, assuming all other nodes stay up.

Checks: ` + strings.Join(checks.Names(), ", ") + `

Exit codes: 0 all evaluated nodes are drainable, 1 at least one node is not
drainable, 2 the analysis itself failed.`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          run,
}

func init() {
	cobra.OnInitialize(initConfig)

	flags := rootCmd.PersistentFlags()
	flags.StringVar(&configFile, "config", "", "config file (default: ~/.config/drainok/config.yaml)")
	flags.String("kubeconfig", "", "path to the kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	flags.String("context", "", "kubeconfig context to use")
	flags.StringP("output", "o", "table", "output format: table, json or yaml")
	flags.StringSlice("ignore-checks", nil, "comma-separated checks to skip ("+strings.Join(checks.Names(), ", ")+")")
	flags.Bool("include-control-plane", false, "evaluate control-plane nodes instead of skipping them")

	for _, name := range []string{"kubeconfig", "context", "output", "ignore-checks", "include-control-plane"} {
		if err := viper.BindPFlag(name, flags.Lookup(name)); err != nil {
			panic(err)
		}
	}
}

func initConfig() {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", "drainok"))
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}
	viper.SetEnvPrefix("DRAINOK")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && configFile != "" {
			fmt.Fprintf(os.Stderr, "warning: could not read config file: %v\n", err)
		}
	}
}

func run(cmd *cobra.Command, args []string) error {
	client, err := kube.NewClient(viper.GetString("kubeconfig"), viper.GetString("context"))
	if err != nil {
		return err
	}
	snap, err := kube.FetchSnapshot(cmd.Context(), client)
	if err != nil {
		return err
	}
	results, err := analyzer.Analyze(snap, analyzer.Options{
		NodeNames:           args,
		IgnoreChecks:        splitCommaList(viper.GetStringSlice("ignore-checks")),
		IncludeControlPlane: viper.GetBool("include-control-plane"),
	})
	if err != nil {
		return err
	}
	if err := output.Render(os.Stdout, viper.GetString("output"), results); err != nil {
		return err
	}
	for _, result := range results {
		if !result.Skipped && !result.Drainable {
			return ErrNotDrainable
		}
	}
	return nil
}

// splitCommaList flattens values like ["pdb,fit"] coming from env vars, where
// viper does not split on commas the way pflag does.
func splitCommaList(values []string) []string {
	var result []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

// SetVersionInfo wires the goreleaser build metadata into the CLI.
func SetVersionInfo(version, commit, date string) {
	rootCmd.Version = fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}

// Execute runs the CLI and maps errors to exit codes.
func Execute() int {
	err := rootCmd.Execute()
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrNotDrainable) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	return 2
}
