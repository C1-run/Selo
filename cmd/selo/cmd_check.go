package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/C1-run/selo/internal/gatechain"
	"github.com/spf13/cobra"
)

var (
	checkStepsFile string
	checkFormat    string
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run GateChain compliance check on a steps file",
	RunE:  runCheckCmd,
}

func init() {
	checkCmd.Flags().StringVar(&checkStepsFile, "steps-file", "", "Path to steps.json (array of ChainStep)")
	checkCmd.Flags().StringVar(&checkFormat, "format", "json", "Output format: json or text")
	checkCmd.MarkFlagRequired("steps-file")
}

func runCheckCmd(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(checkStepsFile)
	if err != nil {
		return fmt.Errorf("reading steps file: %w", err)
	}

	var steps []gatechain.ChainStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return fmt.Errorf("parsing steps JSON: %w", err)
	}

	summary, err := gatechain.Summarize(steps)
	if err != nil {
		return fmt.Errorf("summarizing steps: %w", err)
	}

	if err := gatechain.ValidateSummary(summary); err != nil {
		return fmt.Errorf("summary validation failed: %w", err)
	}

	verdict := gatechain.ConsumeDecision(summary)

	if err := gatechain.VerifyDecision(verdict); err != nil {
		return fmt.Errorf("decision self-check failed: %w", err)
	}

	switch checkFormat {
	case "json":
		out, _ := json.MarshalIndent(verdict, "", "  ")
		fmt.Println(string(out))
	case "text":
		fmt.Printf("Action: %s\n", verdict.Action)
		fmt.Printf("Final status: %s\n", verdict.FinalStatus)
		fmt.Printf("Stop required: %v\n", verdict.StopRequired)
		fmt.Printf("Review required: %v\n", verdict.ReviewRequired)
		fmt.Printf("Reason: %s\n", verdict.ActionReason)
		if len(verdict.Warnings) > 0 {
			fmt.Println("Warnings:")
			for _, w := range verdict.Warnings {
				fmt.Printf("  - %s\n", w)
			}
		}
		if len(verdict.BlockingTools) > 0 {
			fmt.Println("Blocking tools:")
			for _, t := range verdict.BlockingTools {
				fmt.Printf("  - %s\n", t)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown format %q (use json or text)", checkFormat)
	}
	return nil
}
