package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	re "regexp"
	"strconv"
	s "strings"

	"github.com/subchen/go-log"
)

var dir = cwd()

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
	if gitDir, err = run(fmt.Sprintf("git -C %s rev-parse --git-dir", dir)); err != nil {
		return 0
	}
	if stash, err = ioutil.ReadFile(fmt.Sprintf("%s/logs/refs/stash", gitDir)); err != nil {
		return 0
	}
	_ = stash
	return 0
}

/*
def get_stash():
    """Execute git command to get stash info."""
    cmd = Popen(["git", "rev-parse", "--git-dir"], stdout=PIPE, stderr=PIPE)
    so, se = cmd.communicate()
    stash_file = "{}{}".format(so.decode("utf-8").rstrip(), "/logs/refs/stash")

    try:
        with open(stash_file) as f:
            return sum(1 for _ in f)
    except IOError:
        return 0
*/

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
		Untracked:  untracked,
		Insertions: insertions,
		Deletions:  deletions,
	}
}

func main() {
	log.Default.Level = log.DEBUG
	r := parseStatus()
	log.Infoln("=== PARSED STATUS ===")
	log.Infof("    Branch: %s", r.Branch)
	log.Infof("    Remote: %s", r.Remote)
	log.Infof("     Added: %d", r.Added)
	log.Infof("  Modified: %d", r.Modified)
	log.Infof("   Deleted: %d", r.Deleted)
	log.Infof("   Renamed: %d", r.Renamed)
	log.Infof("  Unmerged: %d", r.Unmerged)
	log.Infof(" Untracked: %d", r.Untracked)
	log.Infof("Insertions: %d", r.Insertions)
	log.Infof(" Deletions: %d", r.Deletions)
}

// vim:set sw=4 ts=4:
