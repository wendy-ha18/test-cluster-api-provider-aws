/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package k8srelease provides the detect-k8s-release CLI command.
package k8srelease

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"sigs.k8s.io/cluster-api-provider-aws/v2/cmd/clusterawsadm/k8srelease"
	cmdout "sigs.k8s.io/cluster-api-provider-aws/v2/cmd/clusterawsadm/printers"
)

var (
	githubToken   string
	outputPrinter string
)

// Cmd returns the detect-k8s-release command.
func Cmd() *cobra.Command {
	newCmd := &cobra.Command{
		Use:   "detect-k8s-release <version|capa>",
		Short: "Detect supported Kubernetes release versions",
		Long: templates.LongDesc(`
			Query the kubernetes/kubernetes GitHub repository for stable release tags
			and report supported versions.

			Pass a minor version (e.g. 1.32) to list all stable patch releases for
			that minor version, or pass "capa" to list the latest three minor versions
			supported by CAPA according to the AMI publication policy.
		`),
		Example: templates.Examples(`
			# List all stable patch releases for Kubernetes 1.32
			clusterawsadm detect-k8s-release 1.32

			# List the latest 3 minor versions supported by CAPA
			clusterawsadm detect-k8s-release capa

			# Output as JSON
			clusterawsadm detect-k8s-release capa --output json

			# Use a GitHub token to avoid API rate limiting
			clusterawsadm detect-k8s-release 1.32 --token $GITHUB_TOKEN
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[0]

			printer, err := cmdout.New(outputPrinter, os.Stdout)
			if err != nil {
				return fmt.Errorf("failed creating output printer: %w", err)
			}

			if input == "capa" {
				return runCapa(printer)
			}

			return runMinor(input, printer)
		},
	}

	newCmd.Flags().StringVar(&githubToken, "token", "", "GitHub personal access token (increases API rate limit)")
	newCmd.Flags().StringVarP(&outputPrinter, "output", "o", "table", "Output format: table, json, or yaml")

	return newCmd
}

// runCapa detects the top 3 CAPA-supported minor versions and prints them.
func runCapa(printer cmdout.Printer) error {
	result, err := k8srelease.DetectSupportedVersions(githubToken)
	if err != nil {
		return err
	}

	if outputPrinter == string(cmdout.PrinterTypeTable) {
		return printer.Print(result.ToTable())
	}
	return printer.Print(result)
}

// runMinor detects all patch releases for a single minor version and prints them.
func runMinor(input string, printer cmdout.Printer) error {
	parts := strings.SplitN(input, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid version %q: expected format MAJOR.MINOR (e.g. 1.32) or \"capa\"", input)
	}

	result, err := k8srelease.DetectVersionsForMinor(input, githubToken)
	if err != nil {
		return err
	}

	if outputPrinter == string(cmdout.PrinterTypeTable) {
		return printer.Print(result.ToTable())
	}
	return printer.Print(result)
}
