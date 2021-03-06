//
// In this file we're not checking for a few things, e.g.:
// - BaseFee
// - Signature validation
// - Amount == 0
//
// Those are part of `IsWellFormed` because they can be checked without context
// Not that when a fail condition is tested, the test is made to pass afterwards
// to ensure the error happened because of the expected cause, and not as a side
// effect of something else being broken
//
package runner

import (
	"testing"

	"boscoin.io/sebak/lib/block"
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/common/keypair"
	"boscoin.io/sebak/lib/errors"
	"boscoin.io/sebak/lib/storage"
	"boscoin.io/sebak/lib/transaction"
	"boscoin.io/sebak/lib/transaction/operation"
	"boscoin.io/sebak/lib/voting"

	"github.com/stretchr/testify/require"
)

// Test with some missing block accounts
func TestValidateTxPaymentMissingBlockAccount(t *testing.T) {
	kps := keypair.Random()
	kpt := keypair.Random()

	st := storage.NewTestStorage()
	defer st.Close()

	tx := transaction.Transaction{
		H: transaction.Header{
			Version: common.TransactionVersionV1,
			Created: common.NowISO8601(),
		},
		B: transaction.Body{
			Source:     kps.Address(), // Need a well-formed address
			Fee:        common.BaseFee,
			SequenceID: 0,
			Operations: []operation.Operation{
				operation.Operation{
					H: operation.Header{Type: operation.TypePayment},
					B: operation.Payment{Target: kpt.Address(), Amount: common.Amount(10000)},
				},
			},
		},
	}
	tx.H.Hash = tx.B.MakeHashString()
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.BlockAccountDoesNotExists)

	// Now add the source account but not the target
	bas := block.BlockAccount{
		Address: kps.Address(),
		Balance: common.Amount(1 * common.AmountPerCoin),
	}
	bas.MustSave(st)
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.BlockAccountDoesNotExists)

	// Now just the target
	st1 := storage.NewTestStorage()
	defer st1.Close()
	bat := block.BlockAccount{
		Address: kpt.Address(),
		Balance: common.Amount(1 * common.AmountPerCoin),
	}
	bat.MustSave(st1)
	require.Equal(t, ValidateTx(st1, common.Config{}, tx), errors.BlockAccountDoesNotExists)

	// And finally, bot
	st2 := storage.NewTestStorage()
	defer st2.Close()
	bas.MustSave(st2)
	bat.MustSave(st2)
	require.Nil(t, ValidateTx(st2, common.Config{}, tx))
}

// Check for correct sequence ID
func TestValidateTxWrongSequenceID(t *testing.T) {
	kps := keypair.Random()
	kpt := keypair.Random()

	st := storage.NewTestStorage()
	defer st.Close()
	bas := block.BlockAccount{
		Address:    kps.Address(),
		Balance:    common.Amount(1 * common.AmountPerCoin),
		SequenceID: 1,
	}
	bat := block.BlockAccount{
		Address: kpt.Address(),
		Balance: common.Amount(1 * common.AmountPerCoin),
	}
	bas.MustSave(st)
	bat.MustSave(st)

	tx := transaction.Transaction{
		H: transaction.Header{
			Version: common.TransactionVersionV1,
			Created: common.NowISO8601(),
		},
		B: transaction.Body{
			Source:     kps.Address(),
			Fee:        common.BaseFee,
			SequenceID: 0,
			Operations: []operation.Operation{
				operation.Operation{
					H: operation.Header{Type: operation.TypePayment},
					B: operation.Payment{Target: kpt.Address(), Amount: common.Amount(10000)},
				},
			},
		},
	}
	tx.H.Hash = tx.B.MakeHashString()
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.TransactionInvalidSequenceID)
	tx.B.SequenceID = 2
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.TransactionInvalidSequenceID)
	tx.B.SequenceID = 1
	require.Nil(t, ValidateTx(st, common.Config{}, tx))
}

// Check sending the whole balance
func TestValidateTxOverBalance(t *testing.T) {
	kps := keypair.Random()
	kpt := keypair.Random()

	st := storage.NewTestStorage()
	defer st.Close()
	bas := block.BlockAccount{
		Address:    kps.Address(),
		Balance:    common.Amount(1 * common.AmountPerCoin),
		SequenceID: 1,
	}
	bat := block.BlockAccount{
		Address: kpt.Address(),
		Balance: common.Amount(1 * common.AmountPerCoin),
	}
	bas.MustSave(st)
	bat.MustSave(st)

	opbody := operation.Payment{Target: kpt.Address(), Amount: bas.Balance}
	tx := transaction.Transaction{
		H: transaction.Header{
			Version: common.TransactionVersionV1,
			Created: common.NowISO8601(),
		},
		B: transaction.Body{
			Source:     kps.Address(),
			Fee:        common.BaseFee,
			SequenceID: 1,
			Operations: []operation.Operation{
				operation.Operation{
					H: operation.Header{Type: operation.TypePayment},
					B: opbody,
				},
			},
		},
	}
	tx.H.Hash = tx.B.MakeHashString()
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.TransactionExcessAbilityToPay)
	opbody.Amount = bas.Balance.MustSub(common.BaseFee)
	tx.B.Operations[0].B = opbody
	require.Nil(t, ValidateTx(st, common.Config{}, tx))

	// Also test multiple operations
	// Note: The account balance is 1 BOS (10M units), so we make 4 ops of 2,5M
	// and check that the BaseFee are correctly calculated
	op := tx.B.Operations[0]
	opbody.Amount = common.Amount(2500000)
	op.B = opbody
	tx.B.Operations = []operation.Operation{op, op, op, op}
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.TransactionExcessAbilityToPay)

	// Now the total amount of the ops + balance is equal to the balance
	opbody.Amount = opbody.Amount.MustSub(common.BaseFee.MustMult(len(tx.B.Operations)))
	tx.B.Operations[0].B = opbody
	tx.B.Fee = common.BaseFee * 4
	require.Nil(t, ValidateTx(st, common.Config{}, tx))
}

// Test creating an already existing account
func TestValidateOpCreateExistsAccount(t *testing.T) {
	kps := keypair.Random()
	kpt := keypair.Random()

	st := storage.NewTestStorage()
	defer st.Close()

	bas := block.BlockAccount{
		Address: kps.Address(),
		Balance: common.Amount(1 * common.AmountPerCoin),
	}
	bat := block.BlockAccount{
		Address: kpt.Address(),
		Balance: common.Amount(1 * common.AmountPerCoin),
	}
	bat.MustSave(st)
	bas.MustSave(st)

	tx := transaction.Transaction{
		H: transaction.Header{
			Version: common.TransactionVersionV1,
			Created: common.NowISO8601(),
		},
		B: transaction.Body{
			Source:     kps.Address(), // Need a well-formed address
			Fee:        common.BaseFee,
			SequenceID: 0,
			Operations: []operation.Operation{
				operation.Operation{
					H: operation.Header{Type: operation.TypeCreateAccount},
					B: operation.CreateAccount{Target: kpt.Address(), Amount: common.Amount(10000)},
				},
			},
		},
	}
	tx.H.Hash = tx.B.MakeHashString()
	require.Equal(t, ValidateTx(st, common.Config{}, tx), errors.BlockAccountAlreadyExists)

	st1 := storage.NewTestStorage()
	defer st1.Close()
	bas.MustSave(st1)
	require.Nil(t, ValidateTx(st1, common.Config{}, tx))
}

func TestOpsInBalotLimit(t *testing.T) {
	var checkerFuncs = []common.CheckerFunc{
		IsNew,
		CheckMissingTransaction,
		BallotTransactionsOperationLimit,
	}

	{ // with empty transactions; no error :)
		var txs []string
		config := common.NewTestConfig()
		nr := createTestNodeRunner(1, config)[0]

		checker := &BallotTransactionChecker{
			DefaultChecker:   common.DefaultChecker{Funcs: checkerFuncs},
			NodeRunner:       nr,
			Conf:             nr.Conf,
			LocalNode:        nr.Node(),
			Transactions:     txs,
			VotingHole:       voting.NOTYET,
			transactionCache: NewTransactionCache(nr.Storage(), nr.TransactionPool),
		}

		err := common.RunChecker(checker, common.DefaultDeferFunc)
		require.NoError(t, err)
	}

	{ // with safe number of operations; no error :)
		limit := 100

		config := common.NewTestConfig()
		config.OpsInBallotLimit = limit
		nr := createTestNodeRunner(1, config)[0]

		var txs []string
		_, tx := transaction.TestMakeTransaction(networkID, limit/2)
		nr.TransactionPool.Add(tx)
		txs = append(txs, tx.GetHash())
		_, tx = transaction.TestMakeTransaction(networkID, limit-len(tx.B.Operations))
		nr.TransactionPool.Add(tx)
		txs = append(txs, tx.GetHash())

		checker := &BallotTransactionChecker{
			DefaultChecker:   common.DefaultChecker{Funcs: checkerFuncs},
			NodeRunner:       nr,
			Conf:             nr.Conf,
			LocalNode:        nr.Node(),
			Transactions:     txs,
			VotingHole:       voting.NOTYET,
			transactionCache: NewTransactionCache(nr.Storage(), nr.TransactionPool),
		}

		err := common.RunChecker(checker, common.DefaultDeferFunc)
		require.NoError(t, err)
	}

	{ // with over number of operations; no error :)
		limit := 100

		config := common.NewTestConfig()
		config.OpsInBallotLimit = limit
		nr := createTestNodeRunner(1, config)[0]

		var txs []string
		_, tx := transaction.TestMakeTransaction(networkID, limit/2)
		nr.TransactionPool.Add(tx)
		txs = append(txs, tx.GetHash())
		_, tx = transaction.TestMakeTransaction(networkID, limit-len(tx.B.Operations))
		nr.TransactionPool.Add(tx)
		txs = append(txs, tx.GetHash())
		_, tx = transaction.TestMakeTransaction(networkID, 1)
		nr.TransactionPool.Add(tx)
		txs = append(txs, tx.GetHash())

		checker := &BallotTransactionChecker{
			DefaultChecker:   common.DefaultChecker{Funcs: checkerFuncs},
			NodeRunner:       nr,
			Conf:             nr.Conf,
			LocalNode:        nr.Node(),
			Transactions:     txs,
			VotingHole:       voting.NOTYET,
			transactionCache: NewTransactionCache(nr.Storage(), nr.TransactionPool),
		}

		err := common.RunChecker(checker, common.DefaultDeferFunc)
		require.Equal(t, errors.BallotHasOverMaxOperationsInBallot, err)
	}
}
