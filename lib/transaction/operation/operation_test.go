package operation

import (
	"encoding/json"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/require"

	"boscoin.io/sebak/lib/common"
)

func TestMakeHashOfOperationBodyPayment(t *testing.T) {
	kp := keypair.Master("find me")

	opb := Payment{
		Target: kp.Address(),
		Amount: common.Amount(100),
	}
	op := Operation{
		H: Header{Type: TypePayment},
		B: opb,
	}
	hashed := op.MakeHashString()

	expected := "24V5mcAAoUX1oSn7pqUgZPGN7MxWVtRxZQ9Pc3yn1SmD"
	require.Equal(t, hashed, expected)
}

func TestIsWellFormedOperation(t *testing.T) {
	op := TestMakeOperation(-1)
	err := op.IsWellFormed(networkID, common.NewConfig())
	require.Nil(t, err)
}

func TestIsWellFormedOperationLowerAmount(t *testing.T) {
	obp := TestMakeOperationBodyPayment(0)
	err := obp.IsWellFormed(networkID, common.NewConfig())
	require.NotNil(t, err)
}

func TestSerializeOperation(t *testing.T) {
	op := TestMakeOperation(-1)
	b, err := op.Serialize()
	require.Nil(t, err)
	require.Equal(t, len(b) > 0, true)

	var o Operation
	err = json.Unmarshal(b, &o)
	require.Nil(t, err)
}

func TestOperationBodyCongressVoting(t *testing.T) {
	opb := CongressVoting{
		Contract: []byte("dummy contract"),
		Voting: struct {
			Start uint64
			End   uint64
		}{
			Start: 1,
			End:   100,
		},
	}
	op := Operation{
		H: Header{Type: TypeCongressVoting},
		B: opb,
	}
	hashed := op.MakeHashString()

	expected := "4CcZvkNYQUgvdmjGDuMx7tesCdRp3HU4CW3pbRxeqtEZ"
	require.Equal(t, hashed, expected)

	err := op.IsWellFormed(networkID, common.NewConfig())
	require.Nil(t, err)

}

func TestOperationBodyCongressVotingResult(t *testing.T) {
	opb := CongressVotingResult{
		BallotStamps: struct {
			Hash string
			Urls []string
		}{
			Hash: string(common.MakeHash([]byte("dummydummy"))),
			Urls: []string{"http://www.boscoin.io/1", "http://www.boscoin.io/2"},
		},
		Voters: struct {
			Hash string
			Urls []string
		}{
			Hash: string(common.MakeHash([]byte("dummydummy"))),
			Urls: []string{"http://www.boscoin.io/3", "http://www.boscoin.io/4"},
		},
		Result: struct {
			Count uint64
			Yes   uint64
			No    uint64
			ABS   uint64
		}{
			Count: 9,
			Yes:   2,
			No:    3,
			ABS:   4,
		},
	}
	op := Operation{
		H: Header{Type: TypeCongressVotingResult},
		B: opb,
	}
	hashed := op.MakeHashString()

	expected := "8DgD3heMuNLYhnNBgPSBEquAdKXuogrSybdqt7WD87CV"
	require.Equal(t, hashed, expected)

	err := op.IsWellFormed(networkID, common.NewConfig())
	require.Nil(t, err)

}
