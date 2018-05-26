package sebak

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/btcsuite/btcutil/base58"
	"github.com/spikeekips/sebak/lib/error"
	"github.com/spikeekips/sebak/lib/storage"
	"github.com/spikeekips/sebak/lib/util"
	"github.com/stellar/go/keypair"
)

type BallotData struct {
	//Hash    string      `json:"hash"`
	Data interface{} `json:"message"` // if `BallotStateINIT` must have the original `Message`
}

func (bd BallotData) IsEmpty() bool {
	return bd.Data == nil
}

func (bd BallotData) Message() util.Message {
	if bd.IsEmpty() {
		return nil
	}

	return bd.Data.(util.Message)
}

func (bd BallotData) Serialize() ([]byte, error) {
	if bd.Data == nil {
		return []byte{}, nil
	}
	return json.Marshal(bd)
}

func (bd BallotData) String() string {
	if bd.Data == nil {
		return ""
	}
	encoded, _ := json.MarshalIndent(bd, "", "  ")
	return string(encoded)
}

type Ballot struct {
	T string
	H BallotHeader
	B BallotBody
	D BallotData
}

func (b Ballot) Clone() Ballot {
	body := BallotBody{
		Hash:       b.B.Hash,
		NodeKey:    b.B.NodeKey,
		State:      b.B.State,
		VotingHole: b.B.VotingHole,
	}
	return Ballot{
		T: b.T,
		H: BallotHeader{
			Hash:      b.H.Hash,
			Signature: b.H.Signature,
		},
		B: body,
		D: b.D,
	}
}

func (b Ballot) Serialize() (encoded []byte, err error) {
	if b.State() == BallotStateINIT {
		encoded, err = json.Marshal(b)
		return
	}

	newBallot := b
	newBallot.D.Data = nil
	encoded, err = json.Marshal(newBallot)

	return
}

func (b Ballot) String() string {
	encoded, _ := json.MarshalIndent(b, "", "  ")
	return string(encoded)
}

// NewBallotFromMessage creates `Ballot` from `Message`. It needs to be
// `Ballot.IsWellFormed()` and `Ballot.Validate()`.
func NewBallotFromMessage(nodeKey string, m util.Message) (ballot Ballot, err error) {
	body := BallotBody{
		Hash:       m.GetHash(),
		NodeKey:    nodeKey,
		State:      InitialState,
		VotingHole: VotingNOTYET,
	}
	data := BallotData{
		Data: m,
	}
	ballot = Ballot{
		T: "ballot",
		H: BallotHeader{
			Hash:      base58.Encode(body.MakeHash()),
			Signature: "",
		},
		B: body,
		D: data,
	}

	return
}

func NewBallotFromJSON(b []byte) (ballot Ballot, err error) {
	if err = json.Unmarshal(b, &ballot); err != nil {
		return
	}

	if ballot.Data().IsEmpty() {
		return
	}

	a, _ := ballot.Data().Serialize()

	// TODO BallotMessage should load message by it's `GetType()`
	tx, _ := NewTransactionFromJSON(a)
	ballot.SetMessage(tx)
	//if err = ballot.IsWellFormed(); err != nil {
	//	return
	//}

	return
}

var BallotWellFormedCheckerFuncs = []util.CheckerFunc{
	checkBallotEmptyNodeKey,
	checkBallotEmptyHashMatch,
	checkBallotVerifySignature,
	checkBallotNoVoting,
	checkBallotHasMessage,
	checkBallotValidState,
}

func (b Ballot) IsWellFormed() (err error) {
	if _, err = util.Checker(context.Background(), BallotWellFormedCheckerFuncs...)(b); err != nil {
		return
	}

	//if err = b.Message().IsWellFormed(); err != nil {
	//	return
	//}

	return
}

func (b Ballot) VerifySignature() (err error) {
	err = keypair.MustParse(b.B.NodeKey).Verify(
		[]byte(b.GetHash()),
		base58.Decode(b.H.Signature),
	)
	if err != nil {
		return sebakerror.ErrorSignatureVerificationFailed
	}

	return
}

func (b Ballot) Validate(st *storage.LevelDBBackend) (err error) {
	return
}

func (b Ballot) GetType() string {
	return b.T
}

func (b Ballot) Equal(m util.Message) bool {
	return b.H.Hash == m.GetHash()
}

func (b Ballot) GetHash() string {
	return b.H.Hash
}

func (b Ballot) MessageHash() string {
	return b.B.Hash
}

func (b Ballot) Data() BallotData {
	return b.D
}

func (b *Ballot) SetMessage(m util.Message) {
	b.D.Data = m
}

func (b Ballot) State() BallotState {
	return b.B.State
}

func (b *Ballot) SetState(state BallotState) {
	b.B.State = state

	return
}

func (b *Ballot) Sign(kp *keypair.Full) {
	if kp.Address() != b.B.NodeKey {
		b.B.NodeKey = kp.Address()
	}

	b.UpdateHash()
	signature, _ := kp.Sign([]byte(b.GetHash()))

	b.H.Signature = base58.Encode(signature)
	return
}

func (b *Ballot) UpdateHash() {
	b.H.Hash = base58.Encode(b.B.MakeHash())

	return
}

func (b *Ballot) Vote(v VotingHole) {
	b.B.VotingHole = v

	return
}

type BallotHeader struct {
	Hash      string `json:"ballot_hash"`
	Signature string `json:"signature"`
}

type BallotBody struct {
	Hash       string      `json:"hash"`
	NodeKey    string      `json:"node_key"` // validator's public address
	State      BallotState `json:"state"`
	VotingHole VotingHole  `json:"voting_hole"`
	Reason     string      `json:"reason"`
}

func (bb BallotBody) MakeHash() []byte {
	return util.MustMakeObjectHash(bb)
}

type BallotBoxes struct {
	util.SafeLock

	Results map[ /* `Message.GetHash()`*/ string]*VotingResult

	WaitingBox  *BallotBox
	VotingBox   *BallotBox
	ReservedBox *BallotBox
}

func NewBallotBoxes() *BallotBoxes {
	return &BallotBoxes{
		Results:     map[string]*VotingResult{},
		WaitingBox:  NewBallotBox(),
		VotingBox:   NewBallotBox(),
		ReservedBox: NewBallotBox(),
	}
}

func (b *BallotBoxes) Len() int {
	return len(b.Results)
}

func (b *BallotBoxes) HasMessage(m util.Message) bool {
	return b.HasMessageByString(m.GetHash())
}

// TODO rename to `HasMessageByHash`
func (b *BallotBoxes) HasMessageByString(hash string) bool {
	_, ok := b.Results[hash]
	return ok
}

func (b *BallotBoxes) VotingResult(ballot Ballot) *VotingResult {
	if !b.HasMessageByString(ballot.MessageHash()) {
		return nil
	}

	return b.Results[ballot.MessageHash()]
}

func (b *BallotBoxes) IsVoted(ballot Ballot) bool {
	vr := b.VotingResult(ballot)
	if vr == nil {
		return false
	}

	return vr.IsVoted(ballot)
}

func (b *BallotBoxes) AddVotingResult(vr *VotingResult, bb *BallotBox) (err error) {
	b.Lock()
	defer b.Unlock()

	b.Results[vr.MessageHash] = vr
	bb.AddVotingResult(vr) // TODO detect error

	return
}

func (b *BallotBoxes) AddBallot(ballot Ballot) (isNew bool, err error) {
	b.Lock()
	defer b.Unlock()

	var vr *VotingResult

	isNew = !b.HasMessageByString(ballot.MessageHash())

	if !isNew {
		vr = b.VotingResult(ballot)
		if err = vr.Add(ballot); err != nil {
			return
		}

		if b.ReservedBox.HasMessageByString(ballot.MessageHash()) {
			b.ReservedBox.RemoveVotingResult(vr) // TODO detect error
			b.VotingBox.AddVotingResult(vr)      // TODO detect error
		}

		return
	}

	vr, err = NewVotingResult(ballot)
	if err != nil {
		return
	}

	// unknown ballot will be in `WaitingBox`
	if ballot.State() == BallotStateINIT {
		err = b.AddVotingResult(vr, b.WaitingBox)
	} else {
		err = b.AddVotingResult(vr, b.VotingBox)
	}

	return
}

type BallotBox struct {
	util.SafeLock

	Hashes sort.StringSlice // `Message.Hash`es
}

func NewBallotBox() *BallotBox {
	return &BallotBox{}
}

func (b *BallotBox) Len() int {
	return len(b.Hashes)
}

func (b *BallotBox) HasMessage(m util.Message) bool {
	return b.HasMessageByString(m.GetHash())
}

func (b *BallotBox) HasMessageByString(hash string) bool {
	return util.InStringArray(b.Hashes, hash)
}

func (b *BallotBox) AddVotingResult(vr *VotingResult) (err error) {
	if b.HasMessageByString(vr.MessageHash) {
		err = sebakerror.ErrorVotingResultAlreadyExists
		return
	}

	b.Lock()
	defer b.Unlock()

	b.Hashes = append(b.Hashes, vr.MessageHash)
	sort.Strings(b.Hashes)

	return
}

func (b *BallotBox) RemoveVotingResult(vr *VotingResult) (err error) {
	if !b.HasMessageByString(vr.MessageHash) {
		err = sebakerror.ErrorVotingResultNotFound
		return
	}

	b.Lock()
	defer b.Unlock()

	i := sort.SearchStrings(b.Hashes, vr.MessageHash)
	b.Hashes = append(b.Hashes[:i], b.Hashes[i+1:]...)
	sort.Strings(b.Hashes)

	return
}
