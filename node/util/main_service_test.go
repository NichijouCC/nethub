package util

import (
	"testing"
)

func TestLeaderVoter1(t *testing.T) {
	voter := NewMainServiceMgr([]string{"http://localhost:2379"})
	voter.StartVote("test_app", "1")
}

func TestLeaderVoter2(t *testing.T) {
	voter := NewMainServiceMgr([]string{"http://localhost:2379"})
	voter.StartVote("test_app", "2")
}

func TestLeaderVoter3(t *testing.T) {
	voter := NewMainServiceMgr([]string{"http://localhost:2379"})
	voter.StartVote("test_app", "3")
}

func TestLeaderWatcher(t *testing.T) {
	voter := NewMainServiceMgr([]string{"http://localhost:2379"})
	voter.StartWatch("test_app")
}
