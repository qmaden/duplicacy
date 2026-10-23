// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gilbertchen/cli"
)

// docs/COMMANDS.md is generated from the command definitions so that it can't drift from the code.  After
// changing commands or options, regenerate it with
//
//	DUPLICACY_UPDATE_DOCS=1 go test -run TestCommandReference ./duplicacy

const commandReferencePath = "../docs/COMMANDS.md"

func flagDetails(flag cli.Flag) (name string, argument string, usage string) {
	switch f := flag.(type) {
	case cli.BoolFlag:
		return f.Name, "", f.Usage
	case cli.BoolTFlag:
		return f.Name, "", f.Usage
	case cli.IntFlag:
		return f.Name, f.Argument, f.Usage
	case cli.StringFlag:
		return f.Name, f.Argument, f.Usage
	case cli.StringSliceFlag:
		return f.Name, f.Argument, f.Usage + " (can be repeated)"
	case cli.IntSliceFlag:
		return f.Name, f.Argument, f.Usage + " (can be repeated)"
	case cli.GenericFlag:
		// Used for tri-state options such as -no-backup, which also accept -no-backup=false
		return f.Name, "[=" + f.Arg + "]", f.Usage
	default:
		return fmt.Sprintf("%v", flag), "", ""
	}
}

func flagTable(flags []cli.Flag) string {
	if len(flags) == 0 {
		return "No options.\n"
	}
	var b strings.Builder
	b.WriteString("| Option | Description |\n|--------|-------------|\n")
	for _, flag := range flags {
		name, argument, usage := flagDetails(flag)
		option := "-" + strings.Split(name, ",")[0]
		if strings.HasPrefix(argument, "[=") {
			option += argument
		} else if argument != "" {
			option += " " + argument
		}
		usage = strings.ReplaceAll(usage, "|", "\\|")
		fmt.Fprintf(&b, "| `%s` | %s |\n", strings.ReplaceAll(option, "|", "\\|"), usage)
	}
	return b.String()
}

func commandReference(app *cli.App) string {
	var b strings.Builder
	b.WriteString("# Command reference\n\n")
	b.WriteString("<!-- Generated from duplicacy/duplicacy_main.go by TestCommandReference; do not edit by hand. -->\n\n")
	b.WriteString("Usage: `duplicacy [global options] <command> [options] [arguments]`\n\n")
	b.WriteString("Run `duplicacy <command> -h` for the same information in the terminal. Passwords and keys can also be ")
	b.WriteString("given through environment variables; see [storage-backends.md](storage-backends.md).\n\n")
	b.WriteString("## Global options\n\n")
	b.WriteString(flagTable(app.Flags))
	b.WriteString("\n## Commands\n\n")
	for _, command := range app.Commands {
		fmt.Fprintf(&b, "- [`%s`](#%s): %s\n", command.Name, command.Name, command.Usage)
	}
	for _, command := range app.Commands {
		fmt.Fprintf(&b, "\n### %s\n\n%s\n\n", command.Name, command.Usage)
		usage := "duplicacy " + command.Name + " [options]"
		if arguments := strings.TrimSpace(command.ArgsUsage); arguments != "" {
			usage += " " + arguments
		}
		fmt.Fprintf(&b, "```\n%s\n```\n\n", usage)
		if command.Description != "" {
			b.WriteString(strings.TrimSpace(command.Description) + "\n\n")
		}
		b.WriteString(flagTable(command.Flags))
	}
	return b.String()
}

func TestCommandReference(t *testing.T) {
	generated := commandReference(newApp())
	if os.Getenv("DUPLICACY_UPDATE_DOCS") != "" {
		if err := os.WriteFile(commandReferencePath, []byte(generated), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(commandReferencePath)
	if err != nil {
		t.Fatalf("%v; generate it with DUPLICACY_UPDATE_DOCS=1 go test -run TestCommandReference ./duplicacy", err)
	}
	if string(committed) != generated {
		t.Errorf("docs/COMMANDS.md is out of date; regenerate it with " +
			"DUPLICACY_UPDATE_DOCS=1 go test -run TestCommandReference ./duplicacy")
	}
}
