// vim:set sw=4 ts=4:
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	s "strings"
)

// Test directories
// var dotDir = os.ExpandEnv("$HOME/dotfiles")
// var vimDir = os.ExpandEnv("$HOME/src/vim")

var dir = cwd()

func run(command string) string {
	cmdArgs := s.Split(command, " ")
	log.Printf("Command: %s", cmdArgs)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
	return string(out)
}

func cwd() string {
	path, err := os.Getwd()
	if err != nil {
		log.Printf("os.Getwd() error: %s", err)
	}
	return path
}

type repoInfo struct {
	Branch    string
	Remote    string
	Added     int
	Modified  int
	Deleted   int
	Renamed   int
	Unmerged  int
	Untracked int
}

func parseStatus() repoInfo {
	status := run(fmt.Sprintf("git -C %s status --porcelain --branch", dir))
	lines := s.Split(status, "\n")
	var branch, remoteBranch string
	var untracked, modified, deleted, renamed, unmerged, added int

	for _, st := range lines[:len(lines)-1] {
		rest := st[2:]
		restParts := s.Split(rest, " ")
		log.Println(st[0:2], rest)

		switch st[0:2] {
		case "##":
			switch {
			case s.Contains(rest, "no branch"):
				branch = run(fmt.Sprintf("git -C %s describe --tags --exact-match", dir))
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
						log.Println(div)
					}
				}
			}
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
	return repoInfo{
		Branch:    branch,
		Remote:    remoteBranch,
		Added:     added,
		Modified:  modified,
		Deleted:   deleted,
		Renamed:   renamed,
		Unmerged:  unmerged,
		Untracked: untracked,
	}
}

/*
# Working Python code
def get_diff():
    """Return +/- (added/deleted) of current repo."""
    cmd = Popen(["git", "diff", "--numstat"], stdout=PIPE, stderr=PIPE)
    stdout, stderr = cmd.communicate()
    raw = stdout.decode("utf-8").splitlines()
    diff = [re.split(r"\s+", r) for r in raw]
    plus = []
    minus = []
    for d in diff:
        plus.append(int(d[0]))
        minus.append(int(d[1]))
    return sum(plus), sum(minus)
*/
func main() {
	r := parseStatus()
	log.Printf("Branch:    %s", r.Branch)
	log.Printf("Remote:    %s", r.Remote)
	log.Printf("Added:     %d", r.Added)
	log.Printf("Modified:  %d", r.Modified)
	log.Printf("Deleted:   %d", r.Deleted)
	log.Printf("Renamed:   %d", r.Renamed)
	log.Printf("Unmerged:  %d", r.Unmerged)
	log.Printf("Untracked: %d", r.Untracked)
	log.Printf("gitDiff:\n%s", run(fmt.Sprintf("git -C %s diff --numstat", dir)))
}
