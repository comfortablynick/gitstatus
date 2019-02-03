package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	re "regexp"
	"strconv"
	s "strings"

	// "github.com/celicoo/docli"
	"github.com/jessevdk/go-flags"
	"github.com/subchen/go-log"
)

var doc = `
Usage: gitstatus [-d] <command> [-h] [-f FORMAT]...

Fast git status for your prompt!

commands:
  h, help               show this help message and exit
  v, version            show version info and exit

optional arguments:
  -f, --format          format output according to string
  -d, --debug           print debug messages
  -t, --timeout         timeout length in ms
  -v, --version         show version info and exit
  -h, --help            show help for command
`
var version = "gitstatus version 0.0.1"

var dir = cwd()

// var dir = os.ExpandEnv("$HOME/git/python")

const gitHashLen = 12

type repoInfo struct {
	Branch     string
	Remote     string
	Added      int
	Modified   int
	Deleted    int
	Renamed    int
	Unmerged   int
	Untracked  int
	Stashed    int
	Insertions int
	Deletions  int
}

func cwd() string {
	path, err := os.Getwd()
	if err != nil {
		log.Debugf("os.Getwd() error: %s", err)
	}
	return path
}

func run(command string) (string, error) {
	cmdArgs := s.Split(command, " ")
	log.Debugf("Command: %s", cmdArgs)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec
	out, err := cmd.Output()
	return string(out), err
}

// Return git tag (if checked out), or else hash
func gitTagOrHash(hashLen int) string {
	var str string
	var err error
	if str, err = run(fmt.Sprintf("git -C %s describe --tags --exact-match", dir)); err == nil {
		return str
	}
	if str, err = run(fmt.Sprintf("git -C %s rev-parse --short=%d HEAD", dir, hashLen)); err == nil {
		return str
	}
	log.Errorf("gitTagOrHash() failed: %s\n", err)
	return ""
}

var space = re.MustCompile(`\s+`)

func gitDiff() (int, int) {
	diff, err := run(fmt.Sprintf("git -C %s diff --numstat", dir))
	if err != nil {
		log.Errorf("gitDiff() failed: %s\n", err)
	}
	difflines := s.Split(diff, "\n")
	var ins, del int
	for _, ln := range difflines[:len(difflines)-1] {
		diffline := space.Split(ln, -1)
		if i, err := strconv.Atoi(diffline[0]); err == nil {
			ins += i
		}
		if i, err := strconv.Atoi(diffline[1]); err == nil {
			del += i
		}
	}
	return ins, del
}

func gitStash() int {
	var gitDir string
	var stash []byte
	var err error
	if gitDir, err = run(fmt.Sprintf("git -C %s rev-parse --show-toplevel", dir)); err != nil {
		return 0
	}
	if stash, err = ioutil.ReadFile(s.Trim(gitDir, "\n") + "/.git/logs/refs/stash"); err != nil {
		return 0
	}
	// Subtract 1 for the last line ending
	return len(s.Split(string(stash), "\n")) - 1
}

func parseBranch(raw string) (string, string) {
	var branch, remoteBranch string
	rest := raw[2:]
	restParts := s.Split(rest, " ")
	log.Debugln(raw[0:2], rest)
	switch {
	case s.Contains(rest, "no branch"):
		branch = gitTagOrHash(gitHashLen)
	case s.Contains(rest, "Initial commit on") || s.Contains(rest, "No commits yet on"):
		branch = restParts[len(restParts)-1]
	case len(s.Split(s.TrimSpace(rest), "...")) == 1:
		branch = s.TrimSpace(rest)
	default:
		splitted := s.Split(s.TrimSpace(rest), "...")
		branch = splitted[0]
		rem := splitted[1]
		switch {
		case len(s.Split(rem, " ")) == 1:
			remoteBranch = s.Split(rem, " ")[0]
		default:
			divergence := s.Join(s.Split(rem, " ")[1:], " ")
			remoteBranch = divergence
			remoteBranch = s.Trim(remoteBranch, "[]")
			for _, div := range s.Split(divergence, ", ") {
				if s.Contains(div, "ahead") {
					log.Debugln("Ahead of remote")
				}
				if s.Contains(div, "behind") {
					log.Debugln("Behind remote")
				}
			}
		}
	}
	return branch, remoteBranch
}

func parseStatus() repoInfo {
	status, err := run(fmt.Sprintf("git -C %s status --porcelain --branch", dir))
	if err != nil {
		log.Errorf("git status failed:\n%s", err)
	}
	lines := s.Split(status, "\n")
	var branch, remoteBranch string
	var untracked, modified, deleted, renamed, unmerged, added, insertions, deletions int

	for _, st := range lines[:len(lines)-1] {
		switch st[0:2] {
		case "##":
			branch, remoteBranch = parseBranch(st)
		default:
			if st[0:2] == "??" {
				untracked++
			}
			if string(st[1]) == "M" {
				modified++
			}
			if string(st[0]) == "U" {
				unmerged++
			}
			if string(st[1]) == "D" {
				deleted++
			}
			if s.Contains(st[0:2], "R") {
				untracked++
			}
			if string(st[0]) != " " {
				added++
			}
		}
	}
	insertions, deletions = gitDiff()
	if remoteBranch == "" {
		remoteBranch = "."
	}
	return repoInfo{
		Branch:     branch,
		Remote:     remoteBranch,
		Added:      added,
		Modified:   modified,
		Deleted:    deleted,
		Renamed:    renamed,
		Unmerged:   unmerged,
		Stashed:    gitStash(),
		Untracked:  untracked,
		Insertions: insertions,
		Deletions:  deletions,
	}
}

type cli struct {
	// Commands
	Help    bool
	Version bool

	// Options
	Debug   bool
	Format  string
	Timeout int16
}

var opts struct {
	Debug   []bool `short:"d" long:"debug" description:"increase debug verbosity"`
	Version bool   `short:"v" long:"version" description:"show version info and exit"`
	Timeout int16  `short:"t" long:"timeout" description:"timeout for git status in ms"`
}

func main() {
	log.Default.Level = log.WARN

	args := (func() []string {
		if len(os.Args) > 1 {
			return os.Args
		}
		// Test args
		return []string{
			"-dd",
		}
	})()

	// Handle args
	extraArgs, err := flags.ParseArgs(&opts, args)
	if err != nil {
		log.Errorln(err)
	}

	switch len(opts.Debug) {
	case 0:
		log.Default.Level = log.WARN
	case 1:
		log.Default.Level = log.INFO
	default:
		log.Default.Level = log.DEBUG
	}
	log.Debugf("Unparsed args:  %v", args)
	log.Debugf("Parsed args:    %+v", opts)
	log.Debugf("Remaining args: %v", extraArgs)
	log.Debugf("Wrote help:     %t\n", flags.WroteHelp(err))
	// args, err := docli.Parse(doc)
	// if err != nil {
	//     log.Fatal(err)
	// }
	// var c cli
	// args.Bind(&c)
	//
	// if c.Debug {
	//     log.Default.Level = log.DEBUG
	// }
	// if len(os.Args) > 1 {
	//     log.Debugf("%+v\n", c)
	// }
	// if c.Help {
	//     fmt.Println(doc)
	//     os.Exit(0)
	// }
	// if c.Version {
	//     fmt.Println(version)
	//     os.Exit(0)
	// }
	// if c.Format != "" {
	//     log.Debugf("Format string: %s", c.Format)
	// }
	// if c.Timeout > 0 {
	//     log.Debugf("Timeout length: %d", c.Timeout)
	// }

	r := parseStatus()
	log.Infoln("=== PARSED STATUS ===")
	log.Infof("    Branch: %s", r.Branch)
	log.Infof("    Remote: %s", r.Remote)
	log.Infof("     Added: %d", r.Added)
	log.Infof("  Modified: %d", r.Modified)
	log.Infof("   Deleted: %d", r.Deleted)
	log.Infof("   Renamed: %d", r.Renamed)
	log.Infof("  Unmerged: %d", r.Unmerged)
	log.Infof("   Stashed: %d", r.Stashed)
	log.Infof(" Untracked: %d", r.Untracked)
	log.Infof("Insertions: %d", r.Insertions)
	log.Infof(" Deletions: %d", r.Deletions)
}

// vim:set sw=4 ts=4:
