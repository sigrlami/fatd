// Copyright © 2019 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"github.com/posener/complete"
	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "balance|chains|transactions",
	Long: `Get information about a FAT Chain.

Query fatd for information about FAT chains that are being tracked.
`,
	//Run: func(cmd *cobra.Command, args []string) {
	//	fmt.Println("get called")
	//},
}

var getCmplCmd = complete.Command{
	Flags: rootCmplCmd.Flags,
	Sub:   complete.Commands{},
}

func init() {
	rootCmd.AddCommand(getCmd)
	rootCmplCmd.Sub["get"] = getCmplCmd
}