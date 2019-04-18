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
	"fmt"
	"os"

	"github.com/Factom-Asset-Tokens/fatd/factom"
	"github.com/Factom-Asset-Tokens/fatd/srv"
	"github.com/spf13/cobra"
)

var address factom.FAAddress

// balanceCmd represents the balance command
var balanceCmd = &cobra.Command{
	Use:                   "balance ADDRESS",
	DisableFlagsInUseLine: true,
	Short:                 "Get the balance of an address",
	Long: `Get the balance of an address.

Queries fatd for the balance of ADDRESS for the specified FAT Chain.

Required flags: --chainid or --tokenid and --identity`,
	//Args: getBalanceArgs,
	Args:    getBalanceArgs,
	PreRunE: validateChainID,
	Run:     getBalance,
}

func init() {
	getCmd.AddCommand(balanceCmd)
}

func getBalanceArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	if err := address.Set(args[0]); err != nil {
		return err
	}
	return nil
}

func getBalance(cmd *cobra.Command, _ []string) {
	var params srv.ParamsGetBalance
	params.ChainID = &ChainID
	params.Address = &address

	var balance uint64
	if err := FATClient.Request("get-balance", params, &balance); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(balance)
}
