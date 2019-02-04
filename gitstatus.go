package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	re "regexp"
	"strconv"
	s "strings"

	"github.com/jessevdk/go-flags"
	"github.com/subchen/go-log"
)

var version = "gitstatus version 0.0.1"

var dir = cwd()

// var dir = os.ExpandEnv("$HOME/git/python")

const gitHashLen = 12

var logLevels = []log.Level{
	log.WARN,
	log.INFO,
	log.DEBUG,
}

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

var reSpace = re.MustCompile(`\s+`)

func gitDiff() (int, int) {
	diff, err := run(fmt.Sprintf("git -C %s diff --numstat", dir))
	if err != nil {
		log.Errorf("gitDiff() failed: %s\n", err)
	}
	difflines := s.Split(diff, "\n")
	var ins, del int
	for _, ln := range difflines[:len(difflines)-1] {
		diffline := reSpace.Split(ln, -1)
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

// Options defines command line arguments
var Options struct {
	Debug   []bool `short:"d" long:"debug" description:"increase debug verbosity"`
	Version bool   `short:"v" long:"version" description:"show version info and exit"`
	Timeout int16  `short:"t" long:"timeout" description:"timeout for git status in ms" value-name:"timeout_ms"`
	Format  string `short:"f" long:"format" description:"printf-style format string for git prompt" value-name:"FORMAT"`
}

func formatOutput(status []string, fstring string) string {
	return fmt.Sprintf(fstring, status)
}

func main() {
	log.Default.Level = log.WARN

	args := (func() []string {
		if len(os.Args) > 1 {
			return os.Args[1:]
		}
		// Test args
		log.Warnln("Using test arguments")
		return []string{
			"-ddd",
		}
	})()

	var parser = flags.NewParser(&Options, flags.Default)
	extraArgs, err := parser.ParseArgs(args)

	if err != nil {
		if !flags.WroteHelp(err) {
			parser.WriteHelp(os.Stderr)
			os.Exit(1)
		}
	}

	// Get log level
	verbosity, maxLevels := len(Options.Debug), len(logLevels)
	if verbosity > maxLevels-1 {
		verbosity = maxLevels - 1
	}

	log.Default.Level = logLevels[verbosity]

	log.Debugf("Unparsed args:  %v", args)
	log.Debugf("Parsed args:    %+v", Options)
	log.Debugf("Remaining args: %v", extraArgs)

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
