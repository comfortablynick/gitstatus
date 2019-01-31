package main

import (
	"fmt"
	"os"
	"os/exec"
	re "regexp"
	"strconv"
	s "strings"

	"github.com/subchen/go-log"
)

var dir = cwd()

const gitHashLen = 12

func cwd() string {
	path, err := os.Getwd()
	if err != nil {
		log.Debugf("os.Getwd() error: %s", err)
	}
	return path
}

func run(command string) string {
	cmdArgs := s.Split(command, " ")
	log.Debugf("Command: %s", cmdArgs)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec
	out, err := cmd.Output()
	if err != nil {
		log.Warnf("cmd.Run() failed with %s\n", err)
	}
	return string(out)
}

// Return git tag (if checked out), or else hash
func gitTagOrHash(hashLen int) string {
	if tag := run(fmt.Sprintf("git -C %s describe --tags --exact-match", dir)); tag != "" {
		return tag
	}
	return run(fmt.Sprintf("git -C %s rev-parse --short=%d HEAD", dir, hashLen))
}

var space = re.MustCompile(`\s+`)

func gitDiff() (int, int) {
	diff := run(fmt.Sprintf("git -C %s diff --numstat", dir))
	difflines := s.Split(diff, "\n")
	var ins, del int
	for _, ln := range difflines[:len(difflines)-1] {
		diffline := space.Split(ln, -1)
		// log.Println(diffline[1])
		if i, err := strconv.Atoi(diffline[0]); err == nil {
			ins += i
		}
		if i, err := strconv.Atoi(diffline[1]); err == nil {
			del += i
		}
	}
	return ins, del
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
	Insertions int
	Deletions  int
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
				log.Debugln(div)
			}
		}
	}
	return branch, remoteBranch
}

func parseStatus() repoInfo {
	status := run(fmt.Sprintf("git -C %s status --porcelain --branch", dir))
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
		Untracked:  untracked,
		Insertions: insertions,
		Deletions:  deletions,
	}
}

func main() {
	log.Default.Level = log.DEBUG
	r := parseStatus()
	log.Debugln("=== PARSED STATUS ===")
	log.Debugf("    Branch: %s", r.Branch)
	log.Debugf("    Remote: %s", r.Remote)
	log.Debugf("     Added: %d", r.Added)
	log.Debugf("  Modified: %d", r.Modified)
	log.Debugf("   Deleted: %d", r.Deleted)
	log.Debugf("   Renamed: %d", r.Renamed)
	log.Debugf("  Unmerged: %d", r.Unmerged)
	log.Debugf(" Untracked: %d", r.Untracked)
	log.Debugf("Insertions: %d", r.Insertions)
	log.Debugf(" Deletions: %d", r.Deletions)
}

// vim:set sw=4 ts=4:
