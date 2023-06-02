package node

import "testing"

func TestLeaderVoter1(t *testing.T) {
	voter := NewLeaderVoter([]string{"http://localhost:2379"}, "test_app", 1)
	voter.StartVote()
}

func TestLeaderVoter2(t *testing.T) {
	voter := NewLeaderVoter([]string{"http://localhost:2379"}, "test_app", 2)
	voter.StartVote()
}

func TestLeaderVoter3(t *testing.T) {
	voter := NewLeaderVoter([]string{"http://localhost:2379"}, "test_app", 3)
	voter.StartVote()
}

func TestLeaderWatcher(t *testing.T) {
	voter := NewLeaderWatcher([]string{"http://localhost:2379"}, "test_app")
	voter.StartWatch()
}
