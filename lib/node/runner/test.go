package runner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"boscoin.io/sebak/lib/ballot"
	"boscoin.io/sebak/lib/block"
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/common/keypair"
	"boscoin.io/sebak/lib/consensus"
	"boscoin.io/sebak/lib/network"
	"boscoin.io/sebak/lib/node"
	"boscoin.io/sebak/lib/transaction"
	"boscoin.io/sebak/lib/voting"
)

var networkID []byte = []byte("sebak-unittest")

func MakeNodeRunner() (*NodeRunner, *node.LocalNode) {
	conf := common.NewTestConfig()
	_, n, localNode := network.CreateMemoryNetwork(nil)

	policy, _ := consensus.NewDefaultVotingThresholdPolicy(66)

	localNode.AddValidators(localNode.ConvertToValidator())
	connectionManager := network.NewValidatorConnectionManager(localNode, n, policy, conf)

	st := block.InitTestBlockchain()
	is, _ := consensus.NewISAAC(localNode, policy, connectionManager, st, conf, nil)
	tp := transaction.NewPool(conf)
	nodeRunner, _ := NewNodeRunner(localNode, policy, n, is, st, tp, conf)
	return nodeRunner, localNode
}

func GetTransaction() (transaction.Transaction, []byte) {
	kpNewAccount := keypair.Random()

	tx := transaction.MakeTransactionCreateAccount(networkID, block.GenesisKP, kpNewAccount.Address(), common.BaseReserve)
	tx.B.SequenceID = uint64(0)
	tx.Sign(block.GenesisKP, networkID)

	if txByte, err := tx.Serialize(); err != nil {
		panic(err)
	} else {
		return tx, txByte
	}
}

func GetCreateAccountTransaction(sequenceID uint64, amount uint64) (transaction.Transaction, []byte, *keypair.Full) {
	initialBalance := common.Amount(amount)
	kpNewAccount := keypair.Random()
	tx := transaction.MakeTransactionCreateAccount(networkID, block.GenesisKP, kpNewAccount.Address(), initialBalance)
	tx.B.SequenceID = sequenceID
	tx.Sign(block.GenesisKP, networkID)

	if txByte, err := tx.Serialize(); err != nil {
		panic(err)
	} else {
		return tx, txByte, kpNewAccount
	}
}

func GetPaymentTransaction(kpSource *keypair.Full, target string, sequenceID uint64, amount uint64) (transaction.Transaction, []byte) {
	balance := common.Amount(amount)

	tx := transaction.MakeTransactionPayment(networkID, kpSource, target, balance)
	tx.B.SequenceID = sequenceID
	tx.Sign(kpSource, networkID)

	if txByte, err := tx.Serialize(); err != nil {
		panic(err)
	} else {
		return tx, txByte
	}
}

func GenerateBallot(proposer *node.LocalNode, basis voting.Basis, tx transaction.Transaction, ballotState ballot.State, sender *node.LocalNode, conf common.Config) *ballot.Ballot {
	b := ballot.NewBallot(sender.Address(), proposer.Address(), basis, []string{tx.GetHash()})
	b.SetVote(ballot.StateINIT, voting.YES)

	opi, _ := ballot.NewInflationFromBallot(*b, block.CommonKP.Address(), common.BaseReserve)
	opc, _ := ballot.NewCollectTxFeeFromBallot(*b, block.CommonKP.Address(), tx)
	ptx, _ := ballot.NewProposerTransactionFromBallot(*b, opc, opi)
	b.SetProposerTransaction(ptx)
	b.Sign(proposer.Keypair(), networkID)

	b.SetVote(ballotState, voting.YES)
	b.Sign(sender.Keypair(), networkID)

	if err := b.IsWellFormed(conf); err != nil {
		panic(err)
	}

	return b
}

func GenerateEmptyTxBallot(proposer *node.LocalNode, basis voting.Basis, ballotState ballot.State, sender *node.LocalNode, conf common.Config) *ballot.Ballot {
	b := ballot.NewBallot(sender.Address(), proposer.Address(), basis, []string{})
	b.SetVote(ballot.StateINIT, voting.YES)

	opi, _ := ballot.NewInflationFromBallot(*b, block.CommonKP.Address(), common.BaseReserve)
	opc, _ := ballot.NewCollectTxFeeFromBallot(*b, block.CommonKP.Address())
	ptx, _ := ballot.NewProposerTransactionFromBallot(*b, opc, opi)
	b.SetProposerTransaction(ptx)
	b.Sign(proposer.Keypair(), networkID)

	b.SetVote(ballotState, voting.YES)
	b.Sign(sender.Keypair(), networkID)

	if err := b.IsWellFormed(conf); err != nil {
		panic(err)
	}

	return b
}

func ReceiveBallot(nodeRunner *NodeRunner, ballot *ballot.Ballot) error {
	data, err := ballot.Serialize()
	if err != nil {
		panic(err)
	}

	ballotMessage := common.NetworkMessage{Type: common.BallotMessage, Data: data}
	return nodeRunner.handleBallotMessage(ballotMessage)
}

func createNodeRunnerForTesting(n int, conf common.Config, recv chan struct{}) (*NodeRunner, []*node.LocalNode, *TestConnectionManager) {
	var ns []*network.MemoryNetwork
	var net *network.MemoryNetwork
	var nodes []*node.LocalNode
	for i := 0; i < n; i++ {
		_, s, v := network.CreateMemoryNetwork(net)
		net = s
		ns = append(ns, s)
		nodes = append(nodes, v)
	}

	for j := 0; j < n; j++ {
		nodes[0].AddValidators(nodes[j].ConvertToValidator())
	}

	localNode := nodes[0]
	policy, _ := consensus.NewDefaultVotingThresholdPolicy(66)

	connectionManager := NewTestConnectionManager(
		localNode,
		ns[0],
		policy,
		recv,
	)

	st := block.InitTestBlockchain()
	is, _ := consensus.NewISAAC(localNode, policy, connectionManager, st, common.NewTestConfig(), nil)
	is.SetProposerSelector(FixedSelector{localNode.Address()})
	tp := transaction.NewPool(conf)

	nr, err := NewNodeRunner(localNode, policy, ns[0], is, st, tp, conf)
	if err != nil {
		panic(err)
	}
	nr.isaacStateManager.blockTimeBuffer = 0

	return nr, nodes, connectionManager
}

func MakeConsensusAndBlock(t *testing.T, tx transaction.Transaction, nr *NodeRunner, nodes []*node.LocalNode, proposer *node.LocalNode) (block.Block, error) {
	nr.TransactionPool.AddFromNode(tx)

	// Generate proposed ballot in nodeRunner
	round := uint64(0)
	_, err := nr.proposeNewBallot(round)
	require.NoError(t, err)

	b := nr.Consensus().LatestBlock()
	basis := voting.Basis{
		Round:     round,
		Height:    b.Height,
		BlockHash: b.Hash,
		TotalTxs:  b.TotalTxs,
	}

	conf := common.NewTestConfig()

	// Check that the transaction is in RunningRounds

	ballotSIGN1 := GenerateBallot(proposer, basis, tx, ballot.StateSIGN, nodes[1], conf)
	err = ReceiveBallot(nr, ballotSIGN1)
	require.NoError(t, err)

	ballotSIGN2 := GenerateBallot(proposer, basis, tx, ballot.StateSIGN, nodes[2], conf)
	err = ReceiveBallot(nr, ballotSIGN2)
	require.NoError(t, err)

	rr := nr.Consensus().RunningRounds[basis.Index()]
	require.Equal(t, 2, len(rr.Voted[proposer.Address()].GetResult(ballot.StateSIGN)))

	ballotACCEPT1 := GenerateBallot(proposer, basis, tx, ballot.StateACCEPT, nodes[1], conf)
	err = ReceiveBallot(nr, ballotACCEPT1)
	require.NoError(t, err)

	ballotACCEPT2 := GenerateBallot(proposer, basis, tx, ballot.StateACCEPT, nodes[2], conf)
	err = ReceiveBallot(nr, ballotACCEPT2)

	blk := nr.Consensus().LatestBlock()

	require.Equal(t, proposer.Address(), blk.Proposer)
	require.Equal(t, 1, len(blk.Transactions))
	return blk, err
}
