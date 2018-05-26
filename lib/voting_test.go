package sebak

import (
	"testing"

	"github.com/stellar/go/keypair"
)

func makeBallotsWithSameMessageHash(n int) (kps []*keypair.Full, ballots []Ballot) {
	baseKpNode, _, baseBallot := makeNewBallot(BallotStateINIT, VotingYES)
	kps = append(kps, baseKpNode)
	ballots = append(ballots, baseBallot)

	for i := 0; i < n-1; i++ {
		kpNode, _, ballot := makeNewBallot(BallotStateINIT, VotingYES)
		ballot.B.Hash = baseBallot.MessageHash()
		ballot.UpdateHash()
		ballot.Sign(kpNode)

		kps = append(kps, kpNode)
		ballots = append(ballots, ballot)
	}

	return
}

func TestNewVotingResult(t *testing.T) {
	_, _, ballot := makeNewBallot(BallotStateINIT, VotingYES)

	vr, err := NewVotingResult(ballot)
	if err != nil {
		t.Error(err)
		return
	}
	if len(vr.ID) < 1 {
		t.Error("`VotingResult.ID` is missing")
		return
	}
}

func TestAddVotingResult(t *testing.T) {
	_, _, ballot0 := makeNewBallot(BallotStateINIT, VotingYES)
	kpNode1, _, ballot1 := makeNewBallot(BallotStateINIT, VotingYES)

	vr, _ := NewVotingResult(ballot0)
	if err := vr.Add(ballot1); err == nil {
		t.Error("`VotingResult.Add` must occurr the `ErrorHashDoesNotMatch`")
	}

	ballot1.B.Hash = ballot0.MessageHash()
	ballot1.UpdateHash()
	ballot1.Sign(kpNode1)
	if err := vr.Add(ballot1); err != nil {
		t.Error("failed to `VotingResult.Add`", err)
		return
	}
}

func TestVotingResultCheckThreshold(t *testing.T) {
	var numberOfBallots int = 5
	_, ballots := makeBallotsWithSameMessageHash(numberOfBallots)

	vr, _ := NewVotingResult(ballots[0])
	for _, ballot := range ballots[1:] {
		vr.Add(ballot)
	}

	policy, _ := NewDefaultVotingThresholdPolicy(100, 100, 100)
	policy.SetValidators(numberOfBallots)
	if _, ended := vr.CheckThreshold(BallotStateNONE, policy); ended {
		t.Error("`BallotStateNONE` must be `false`")
		return
	}
	if _, ended := vr.CheckThreshold(BallotStateINIT, policy); !ended {
		t.Error("`BallotStateINIT` must be `true`")
		return
	}
	policy, _ = NewDefaultVotingThresholdPolicy(100, 100, 100)
	policy.SetValidators(numberOfBallots * 2)
	if _, ended := vr.CheckThreshold(BallotStateINIT, policy); ended {
		t.Error("`BallotStateINIT` must be `false`")
		return
	}
}

func TestVotingResultGetResult(t *testing.T) {
	var numberOfBallots int = 5
	_, ballots := makeBallotsWithSameMessageHash(numberOfBallots)

	vr, _ := NewVotingResult(ballots[0])
	for _, ballot := range ballots[1:] {
		vr.Add(ballot)
	}

	{
		policy, _ := NewDefaultVotingThresholdPolicy(100, 30, 30)
		policy.SetValidators(numberOfBallots)

		_, state, ended := vr.MakeResult(policy)
		if !ended {
			t.Error("failed to make agreement")
			return
		}
		if state != BallotStateINIT {
			t.Errorf("state must be `BallotStateINIT`: %v", state)
			return
		}
		if !ended {
			t.Error("must be ended")
			return
		}
	}

	{
		// too high threshold
		policy, _ := NewDefaultVotingThresholdPolicy(100, 50, 50)
		policy.SetValidators(numberOfBallots + 100)

		_, state, ended := vr.MakeResult(policy)
		if ended {
			t.Error("agreement must be failed")
			return
		}
		if state != BallotStateINIT {
			t.Errorf("state must be `BallotStateINIT`: %v", state)
			return
		}
		if ended {
			t.Error("must not be ended")
			return
		}
	}
}

func TestVotingResultGetResultHigherStateMustBePicked(t *testing.T) {
	var numberOfBallots int = 5
	kps, ballots := makeBallotsWithSameMessageHash(numberOfBallots)

	vr, _ := NewVotingResult(ballots[0])
	for _, ballot := range ballots[1:] {
		vr.Add(ballot)
	}

	// move to `BallotStateACCEPT`
	for i, ballot := range ballots {
		ballot.B.State = BallotStateACCEPT
		ballot.UpdateHash()
		ballot.Sign(kps[i])

		vr.Add(ballot)
	}

	{
		policy, _ := NewDefaultVotingThresholdPolicy(100, 50, 50)
		policy.SetValidators(numberOfBallots)

		_, state, ended := vr.MakeResult(policy)
		if state != BallotStateACCEPT {
			t.Error("state must be `BallotStateACCEPT`")
			return
		}
		if !ended {
			t.Error("must be ended")
			return
		}
	}
}