/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package main

// A simple binary for merging PR that match a criteria
// Usage:
//   submit-queue -token=<github-access-token> -user-whitelist=<file> --jenkins-host=http://some.host [-min-pr-number=<number>] [-dry-run] [-once]

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"k8s.io/kubernetes/pkg/util"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	_ = fmt.Print
)

// RefreshWhitelist updates the whitelist, re-getting the list of committers.
func (config *SubmitQueueConfig) RefreshWhitelist() util.StringSet {
	userSet := util.StringSet{}
	userSet.Insert(config.additionalUserWhitelist...)
	if usersWithCommit, err := config.UsersWithCommit(); err != nil {
		glog.Info("Falling back to static committers list.")
		// Use the static list if there was an error getting the list dynamically
		userSet.Insert(config.committerList...)
	} else {
		userSet.Insert(usersWithCommit...)
	}
	config.userWhitelist = userSet
	return userSet
}

func loadWhitelist(file string) ([]string, error) {
	if len(file) == 0 {
		return []string{}, nil
	}
	fp, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	scanner := bufio.NewScanner(fp)
	result := []string{}
	for scanner.Scan() {
		current := scanner.Text()
		if !strings.HasPrefix(current, "#") {
			result = append(result, current)
		}
	}
	return result, scanner.Err()
}

func writeWhitelist(fileName, header string, items []string) error {
	items = append([]string{header}, items...)
	items = append(items, "")
	return ioutil.WriteFile(fileName, []byte(strings.Join(items, "\n")), 0640)
}

func (config *SubmitQueueConfig) doGenCommitters() error {
	c, err := config.UsersWithCommit()
	if err != nil {
		glog.Fatalf("Unable to read committers from github: %v", err)
	}
	if err = writeWhitelist(config.Committers, "# auto-generated by "+os.Args[0]+" gen-committers; manual additions should go in the whitelist", c); err != nil {
		glog.Fatalf("Unable to write committers: %v", err)
	}
	glog.Info("Successfully updated committers file.")

	users, err := loadWhitelist(config.Whitelist)
	if err != nil {
		glog.Fatalf("error loading whitelist; it will not be updated: %v", err)
	}
	existing := util.NewStringSet(c...)
	newUsers := []string{}
	for _, u := range users {
		if existing.Has(u) {
			glog.Infof("%v is a dup, or already a committer. Will remove from whitelist.", u)
			continue
		}
		existing.Insert(u)
		newUsers = append(newUsers, u)
	}
	if err = writeWhitelist(config.Whitelist, "# remove dups with "+os.Args[0]+" gen-committers", newUsers); err != nil {
		glog.Fatalf("Unable to write de-duped whitelist: %v", err)
	}
	glog.Info("Successfully de-duped whitelist.")
	return nil
}

func addWhitelistCommand(root *cobra.Command, config *SubmitQueueConfig) {
	genCommitters := &cobra.Command{
		Use:   "gencommiters",
		Short: "Generate the list of people with commit access",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.PreExecute(); err != nil {
				return err
			}
			return config.doGenCommitters()
		},
	}
	root.PersistentFlags().StringVar(&config.Whitelist, "user-whitelist", "./whitelist.txt", "Path to a whitelist file that contains users to auto-merge.  Required.")
	root.PersistentFlags().StringVar(&config.Committers, "committers", "./committers.txt", "File in which the list of authorized committers is stored; only used if this list cannot be gotten at run time.  (Merged with whitelist; separate so that it can be auto-generated)")
	root.Flags().StringVar(&config.WhitelistOverride, "whitelist-override-label", "ok-to-merge", "Github label, if present on a PR it will be merged even if the author isn't in the whitelist")

	root.AddCommand(genCommitters)
}
