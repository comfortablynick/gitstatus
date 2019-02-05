package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	re "regexp"
	"strconv"
	s "strings"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/subchen/go-log"
)

const version = `
gitstatus version 0.0.1

© 2019 Nicholas Murphy
(github.com/comfortablynick)
`

const gitHashLen = 12

var logLevels = []log.Level{
	log.WARN,
	log.INFO,
	log.DEBUG,
}

// RepoInfo holds extracted data from `git status`
type RepoInfo struct {
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

var repo RepoInfo

func cwd() string {
	path, err := os.Getwd()
	if err != nil {
		log.Debugf("os.Getwd() error: %s", err)
	}
	return path
}

func run(command string) (string, error) {
	cmdArgs := s.Split(command, " ")
	log.Debugf("Command: %s Dir: %s", cmdArgs, Options.Dir)

	// Create context with timeout deadline to stop execution
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(Options.Timeout)*time.Millisecond)
	defer cancel()

	// Run command inside context
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...) // #nosec
	cmd.Dir = Options.Dir
	out, err := cmd.Output()

	if ctx.Err() == context.DeadlineExceeded {
		log.Debugln("Command timed out")
		timeoutReached()
	}
	return string(out), err
}

// Return git tag (if checked out), or else hash
func gitTagOrHash(hashLen int) string {
	var str string
	var err error
	if str, err = run("git describe --tags --exact-match"); err == nil {
		return s.TrimRight(str, "\n")
	}
	if str, err = run(fmt.Sprintf("git rev-parse --short=%d HEAD", hashLen)); err == nil {
		return s.TrimRight(str, "\n")
	}
	log.Errorf("gitTagOrHash() failed: %s\n", err)
	return ""
}

var reSpace = re.MustCompile(`\s+`)

func gitDiff() (int, int) {
	diff, err := run("git diff --numstat")
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

// gitStash extracts the number of stashes in the stack
func gitStash() int {
	var gitDir string
	var stash []byte
	var err error
	if gitDir, err = run("git rev-parse --show-toplevel"); err != nil {
		return 0
	}
	if stash, err = ioutil.ReadFile(s.Trim(gitDir, "\n") + "/.git/logs/refs/stash"); err != nil {
		return 0
	}
	// Subtract 1 for the last line ending
	return len(s.Split(string(stash), "\n")) - 1
}

// parseBranch returns current local branch and remote branch names
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

// parseStatus runs `git status` and parses relevant data
func parseStatus() RepoInfo {
	status, err := run("git status --porcelain --branch")
	if err != nil {
		log.Debugf("git status failed:\n%s", err)
		os.Exit(1)
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
	return RepoInfo{
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

func gitIsDirty(r RepoInfo) bool {
	if r.Modified > 0 || r.Added > 0 || r.Deleted > 0 {
		return true
	}
	return false
}

// Return symbol if bool is true, else empty string
func promptSymbolIfTrue(result bool, symbol string) string {
	if result {
		return symbol
	}
	return ""
}

func formatOutput(r RepoInfo, formatString string) string {
	log.Debugf("Format string: %s", formatString)
	rep := s.NewReplacer(
		"%n", "git",
		"%b", r.Branch,
		"%m", promptSymbolIfTrue(gitIsDirty(r), "+"),
		"%r", r.Remote,
		"%u", fmt.Sprintf("%d", r.Untracked),
		"%a", fmt.Sprintf("%d", r.Added),
		"%d", fmt.Sprintf("%d", r.Deleted),
		"%s", fmt.Sprintf("%d", r.Stashed),
		"%x", fmt.Sprintf("%d", r.Insertions),
		"%y", fmt.Sprintf("%d", r.Deletions),
	)
	return rep.Replace(formatString)
}

// Helper function to write string and exit on timeout
func timeoutReached() {
	log.Debug("Timeout reached; try increasing --timeout param")
	fmt.Println("timeout")
	os.Exit(1)
}

// Options defines command line arguments
var Options struct {
	Verbose []bool `short:"v" long:"verbose" description:"see more debug messages"`
	Version bool   `long:"version" description:"show version info and exit"`
	Dir     string `short:"d" long:"dir" description:"git repo location" value-name:"directory" default:"."`
	Timeout int16  `short:"t" long:"timeout" description:"timeout for git cmds in ms" value-name:"timeout_ms" default:"100"`
	Format  string `short:"f" long:"format" description:"printf-style format string for git prompt" value-name:"FORMAT" default:"[%n:%b]"`
}

func main() {
	log.Default.Level = log.WARN

	// Use cli args if present, else test args
	args := (func() []string {
		if len(os.Args) > 1 {
			return os.Args[1:]
		}
		log.Infoln("Using test arguments")
		return []string{
			"-v",
		}
	})()

	var parser = flags.NewParser(&Options, flags.Default)
	longDesc := `Git status for your prompt, similar to Greg Ward's vcprompt

	Prints according to FORMAT, which may contain:
	%n  show VC name
	%b  show branch
	%r  show remote (default: ".")
	%m  indicate uncomitted changes (modified/added/removed)
	%u  show untracked file count
	%a  show added file count
	%d  show deleted file count
	%s  show stash count
	%x  show insertion count
	%y  show deletion count
	`
	parser.LongDescription = longDesc
	extraArgs, err := parser.ParseArgs(args)

	if err != nil {
		if !flags.WroteHelp(err) {
			parser.WriteHelp(os.Stderr)
		}
		os.Exit(1)
	}

	// Get log level
	verbosity, maxLevels := len(Options.Verbose), len(logLevels)
	if verbosity > maxLevels-1 {
		verbosity = maxLevels - 1
	}

	log.Default.Level = logLevels[verbosity]

	log.Debugf("Raw args:\n%v", args)
	log.Debugf("Parsed args:\n%+v", Options)
	if len(extraArgs) > 0 {
		log.Debugf("Remaining args:\n%v", extraArgs)
	}

	if Options.Version {
		fmt.Println(version)
		os.Exit(0)
	}

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

	// Output formatted string
	fmt.Println(formatOutput(r, Options.Format))
}

// vim:set sw=4 ts=4:
