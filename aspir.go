package pir

import (
	"errors"

	"github.com/ncw/gmp"
	"github.com/sachaservan/paillier"
)

/*
 Single-server AHE variant of ASPIR
*/

// AuthenticatedEncryptedQuery is a single-server encrypted query
// attached with an authentication token that proves knowledge of a
// secret associated with the retrieved item.
// Either Query0 or Query1`is a "null" query that doesn't retrieve
// any value. It is needed to prevent the server from generating
// a ''tagged'' key database in an attempt to learn which item
// is being retrieved by the client ...
type AuthenticatedEncryptedQuery struct {
	Query0         *DoublyEncryptedQuery
	Query1         *DoublyEncryptedQuery
	AuthTokenComm0 *ROCommitment
	AuthTokenComm1 *ROCommitment
}

// AuthenticatedQueryShare contains a secret share of the auth token
// it doesn't need an "equivocation" query like AuthenticatedEncryptedQuery does
// because the verification happens between 2 or more servers
type AuthenticatedQueryShare struct {
	*QueryShare
	AuthToken *AuthTokenShare
}

// AuthQueryPrivateState the client's private state
type AuthQueryPrivateState struct {
	Sk         *paillier.SecretKey
	Bit        int
	AuthToken0 *paillier.Ciphertext
	AuthToken1 *paillier.Ciphertext
}

// ChalToken is the challenge issued to the client
// in order to prove knowledge of the key associated with the retrieved item
type ChalToken struct {
	Token0   *paillier.Ciphertext
	Token1   *paillier.Ciphertext
	SecParam int
}

// ProofToken is the response provided by the client to a ChalToken
type ProofToken struct {
	AuthToken *paillier.Ciphertext
	T         *paillier.Ciphertext
	P         *paillier.DDLEQProof
	QBit      int
	R         *gmp.Int
	S         *gmp.Int
}

// GenerateAuthChalForQuery generates a challenge token for the provided PIR query
func GenerateAuthChalForQuery(
	secparam int,
	keyDB *Database,
	query *AuthenticatedEncryptedQuery,
	nprocs int) (*ChalToken, error) {

	// hack: because ASPIR has group size 1, need to make sure that query is  only retrieving one key
	groupSize := query.Query0.Col.GroupSize
	query.Query0.Col.GroupSize = 1
	query.Query1.Col.GroupSize = 1

	// key database only has one entry per group
	query.Query1.Row.DBWidth /= groupSize
	query.Query0.Row.DBWidth /= groupSize

	// get the row for query0
	rowQueryRes0, err := keyDB.PrivateEncryptedQuery(query.Query0.Row, nprocs)
	if err != nil {
		return nil, err
	}

	// get the row for query0
	rowQueryRes1, err := keyDB.PrivateEncryptedQuery(query.Query1.Row, nprocs)
	if err != nil {
		return nil, err
	}

	res0, err := keyDB.PrivateEncryptedQueryOverEncryptedResult(query.Query0.Col, rowQueryRes0, nprocs)
	if err != nil {
		return nil, err
	}

	res1, err := keyDB.PrivateEncryptedQueryOverEncryptedResult(query.Query1.Col, rowQueryRes1, nprocs)
	if err != nil {
		return nil, err
	}

	// reset to original values
	// TODO: deal with this later
	query.Query0.Col.GroupSize = groupSize
	query.Query1.Col.GroupSize = groupSize
	query.Query0.Row.DBWidth *= groupSize
	query.Query1.Row.DBWidth *= groupSize

	return &ChalToken{res0.Slots[0].Cts[0], res1.Slots[0].Cts[0], secparam}, nil
}

// AuthProve proves that challenge token is correct (a nested encryption of zero)
// bit indicate which query (query0 or query1) is the real query
func AuthProve(state *AuthQueryPrivateState, chalToken *ChalToken) (*ProofToken, error) {

	sk := state.Sk

	var selToken *paillier.Ciphertext
	token0 := sk.NestedSub(chalToken.Token0, state.AuthToken0)
	token1 := sk.NestedSub(chalToken.Token1, state.AuthToken1)

	zero := gmp.NewInt(0)
	decTok0 := sk.NestedDecrypt(token0)
	decTok1 := sk.NestedDecrypt(token1)

	if decTok0.Cmp(zero) != 0 && decTok1.Cmp(zero) != 0 {
		return nil, errors.New("both tokens non-zero -- server likely cheating")
	}

	var chal *paillier.Ciphertext
	var queryBit = state.Bit

	// if one of the tokens is non-zero then the server cheated
	// therefore, we must prove whichever token is zero
	// to avoid leaking information about the original query
	if decTok0.Cmp(zero) != 0 || decTok1.Cmp(zero) != 0 {
		if decTok0.Cmp(zero) == 0 {
			chal = token0
			selToken = state.AuthToken0
			queryBit = 0
		} else {
			chal = token1
			selToken = state.AuthToken1
			queryBit = 1
		}
	} else {
		if state.Bit == 0 {
			chal = token0
			selToken = state.AuthToken0
			queryBit = 0
		} else {
			chal = token1
			selToken = state.AuthToken1
			queryBit = 1
		}
	}

	chal2, a, b := sk.NestedRandomize(chal)

	proof, err := sk.ProveDDLEQ(chalToken.SecParam, chal, chal2, a, b)

	if err != nil {
		return nil, err
	}

	// extract the randomness from the nested ciphertext
	// to prove that ct2 is an encryption of zero
	s := sk.ExtractRandonness(chal2)
	ctInner := sk.DecryptNestedCiphertextLayer(chal2)
	r := sk.ExtractRandonness(ctInner)

	return &ProofToken{selToken, chal2, proof, queryBit, r, s}, nil
}

// AuthCheck verifies the proof provided by the client and outputs True if and only if the proof is valid
func AuthCheck(pk *paillier.PublicKey, query *AuthenticatedEncryptedQuery, chalToken *ChalToken, proofToken *ProofToken) bool {

	var comm *ROCommitment
	var ct1 *paillier.Ciphertext
	if proofToken.QBit == 0 {
		ct1 = chalToken.Token0
		comm = query.AuthTokenComm0
	} else {
		ct1 = chalToken.Token1
		comm = query.AuthTokenComm1
	}

	// perform the subtraction and check commitment
	ct1 = pk.NestedSub(ct1, proofToken.AuthToken)
	if !comm.CheckOpen(ct1.C) {
		return false
	}

	ct2 := proofToken.T

	// make sure that ct2 is a re-encryption of ct1
	if !pk.VerifyDDLEQProof(ct1, ct2, proofToken.P) {
		return false
	}

	// check that ct2 is an encryption of 0 ==> ct1 is an encryption of 0
	// perform a double encryption of zero with provided randomness
	check := pk.EncryptWithRAtLevel(gmp.NewInt(0), proofToken.R, paillier.EncLevelOne)
	check = pk.EncryptWithRAtLevel(check.C, proofToken.S, paillier.EncLevelTwo)

	if check.C.Cmp(ct2.C) != 0 {
		return false
	}

	return true
}

/*
 Secret shared DPF variant of ASPIR
*/

// AuditTokenShare is a secret share of an audit token
// used to authenticate two-server PIR queries
type AuditTokenShare struct {
	T *Slot
}

// AuthTokenShare is a share of the key associated with the queried item
type AuthTokenShare struct {
	T *Slot
}

// NewAuthTokenSharesForKey generates auth token shares for a specific AuthKey (encoded as a slot)
func NewAuthTokenSharesForKey(authKey *Slot, numShares uint) []*AuthTokenShare {

	numBytes := len(authKey.Data)
	shares := make([]*AuthTokenShare, numShares)
	accumulator := NewEmptySlot(numBytes)

	for i := 1; i < int(numShares); i++ {
		share := NewRandomSlot(numBytes)
		XorSlots(accumulator, share)
		shares[i] = &AuthTokenShare{share}
	}

	XorSlots(accumulator, authKey)
	shares[0] = &AuthTokenShare{accumulator}

	return shares
}

// GenerateAuditForSharedQuery generates an audit share that is sent to the other server(s)
func GenerateAuditForSharedQuery(
	keyDB *Database,
	query *AuthenticatedQueryShare,
	nprocs int) (*AuditTokenShare, error) {

	oldGroupSize := query.GroupSize
	query.GroupSize = 1 // key database has group size 1
	bits := keyDB.ExpandSharedQuery(query.QueryShare, nprocs)
	query.GroupSize = oldGroupSize

	return GenerateAuditForSharedQueryWithExpandedBits(keyDB, query, bits, nprocs)
}

// GenerateAuditForSharedQueryWithExpandedBits generates an audit share that is sent to the other server(s)
// using the expanded DPF bits provided to it
func GenerateAuditForSharedQueryWithExpandedBits(
	keyDB *Database,
	query *AuthenticatedQueryShare,
	bits []bool,
	nprocs int) (*AuditTokenShare, error) {

	res, err := keyDB.PrivateSecretSharedQueryWithExpandedBits(query.QueryShare, bits, nprocs)
	if err != nil {
		return nil, err
	}

	if len(res.Shares) != 1 {
		return nil, errors.New("Invalid challenge ciphertext result")
	}

	keySlotShare := res.Shares[0]
	XorSlots(keySlotShare, query.AuthToken.T)
	return &AuditTokenShare{keySlotShare}, nil
}

// CheckAudit outputs True of all provided audit tokens xor to zero
func CheckAudit(auditTokens ...*AuditTokenShare) bool {

	res := NewEmptySlot(len(auditTokens[0].T.Data))
	for _, tok := range auditTokens {
		XorSlots(res, tok.T)
	}

	// make sure the resulting slot is all zero
	if ints, _, _ := res.ToGmpIntArray(1); ints[0].Cmp(gmp.NewInt(0)) != 0 {
		return false
	}

	return true
}
