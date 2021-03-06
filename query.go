package pir

import (
	"math"
	"math/rand"

	"github.com/ncw/gmp"
	"github.com/sachaservan/paillier"
	"github.com/sachaservan/pir/dpf"
)

// QueryShare is a secret share of a query over the database
// to retrieve a row
type QueryShare struct {
	KeyTwoParty    *dpf.Key2P
	KeyMultiParty  *dpf.KeyMP
	PrfKeys        []*dpf.PrfKey
	IsKeywordBased bool
	IsTwoParty     bool
	ShareNumber    uint
	GroupSize      int // height of the database
}

// EncryptedQuery is an encryption of a point function
// that evaluates to 1 at the desired row in the database
// bits = (0, 0,.., 1, ...0, 0)
type EncryptedQuery struct {
	Pk                *paillier.PublicKey
	EBits             []*paillier.Ciphertext
	GroupSize         int
	DBWidth, DBHeight int // if a specific will force these dimentiojs
}

// DoublyEncryptedQuery consists of two encrypted point functions
// that evaluates to 1 at the desired row and column in the database
type DoublyEncryptedQuery struct {
	Row *EncryptedQuery
	Col *EncryptedQuery
}

// NewIndexQueryShares generates PIR query shares for the index
func (dbmd *DBMetadata) NewIndexQueryShares(index int, groupSize int, numShares uint) []*QueryShare {
	return dbmd.newQueryShares(index, groupSize, numShares, true)
}

// NewKeywordQueryShares generates keyword-based PIR query shares for keyword
func (dbmd *DBMetadata) NewKeywordQueryShares(keyword int, groupSize int, numShares uint) []*QueryShare {
	return dbmd.newQueryShares(keyword, groupSize, numShares, false)
}

// NewQueryShares generates random PIR query shares for the index
func (dbmd *DBMetadata) newQueryShares(key int, groupSize int, numShares uint, isIndexQuery bool) []*QueryShare {

	dimHeight := int(math.Ceil(float64(dbmd.DBSize / groupSize))) // need groupSize elements back

	if dimHeight == 0 {
		panic("database height is set to zero; something is wrong")
	}

	// num bits to represent the index
	numBits := uint(math.Log2(float64(dimHeight)) + 1)

	// otherwise assume keyword based (32 bit keys)
	if !isIndexQuery {
		numBits = uint(32)
	}

	pf := dpf.ClientInitialize(numBits)

	var dpfKeysTwoParty []*dpf.Key2P
	var dpfKeysMultiParty []*dpf.KeyMP

	if numShares == 2 {
		dpfKeysTwoParty = pf.GenerateTwoServer(uint(key), 1)
	} else {
		dpfKeysMultiParty = pf.GenerateMultiServer(uint(key), 1, numShares)
	}

	if key >= dimHeight {
		panic("requesting key outside of domain")
	}

	shares := make([]*QueryShare, numShares)
	for i := 0; i < int(numShares); i++ {
		shares[i] = &QueryShare{}
		shares[i].ShareNumber = uint(i)
		shares[i].PrfKeys = pf.PrfKeys
		shares[i].IsKeywordBased = !isIndexQuery
		shares[i].GroupSize = groupSize

		if numShares == 2 {
			shares[i].KeyTwoParty = dpfKeysTwoParty[i]
			shares[i].IsTwoParty = true
		} else {
			shares[i].KeyMultiParty = dpfKeysMultiParty[i]
			shares[i].IsTwoParty = false
		}
	}

	return shares
}

// NewAuthenticatedIndexQueryShares generates PIR query shares for the index
func (dbmd *DBMetadata) NewAuthenticatedIndexQueryShares(
	index int, authKey *Slot, groupSize int, numShares uint) []*AuthenticatedQueryShare {

	queryShares := dbmd.NewIndexQueryShares(index, groupSize, numShares)
	authTokenShares := NewAuthTokenSharesForKey(authKey, numShares)

	authQueryShares := make([]*AuthenticatedQueryShare, numShares)
	for i := 0; i < int(numShares); i++ {
		authQueryShares[i] = &AuthenticatedQueryShare{queryShares[i], authTokenShares[i]}
	}

	return authQueryShares
}

// NewEncryptedQuery generates a new encrypted point function that acts as a PIR query
// defaults to sqrt sized grid database layout
func (dbmd *DBMetadata) NewEncryptedQuery(pk *paillier.PublicKey, groupSize, index int) *EncryptedQuery {

	// compute sqrt dimentions
	height := int(math.Ceil(math.Sqrt(float64(dbmd.DBSize))))
	var width int
	width, height = dbmd.GetDimentionsForDatabase(height, groupSize)

	return dbmd.NewEncryptedQueryWithDimentions(pk, width, height, groupSize, index)
}

// NewEncryptedQueryWithDimentions generates a new encrypted point function that acts as a PIR query
// where the database is viewed as a width x height grid
func (dbmd *DBMetadata) NewEncryptedQueryWithDimentions(pk *paillier.PublicKey, width, height, groupSize, index int) *EncryptedQuery {

	res := make([]*paillier.Ciphertext, height)
	for i := 0; i < height; i++ {
		if i == index {
			res[i] = pk.EncryptOne()
		} else {
			res[i] = pk.EncryptZero()
		}
	}

	return &EncryptedQuery{
		Pk:        pk,
		EBits:     res,
		GroupSize: groupSize,
		DBWidth:   width,
		DBHeight:  height,
	}
}

// NewDoublyEncryptedNullQuery generates a PIR query that does not retrieve any value
func (dbmd *DBMetadata) NewDoublyEncryptedNullQuery(pk *paillier.PublicKey, groupSize int) *DoublyEncryptedQuery {
	return dbmd.NewDoublyEncryptedQuery(pk, groupSize, -1) // index -1 generates the all-zero query
}

// NewDoublyEncryptedQuery generates two encrypted point function that acts as a PIR query
// to select the row and column in the database
func (dbmd *DBMetadata) NewDoublyEncryptedQuery(pk *paillier.PublicKey, groupSize, index int) *DoublyEncryptedQuery {

	// compute sqrt dimentions
	height := int(math.Ceil(math.Sqrt(float64(dbmd.DBSize))))
	var width int
	width, height = dbmd.GetDimentionsForDatabase(height, groupSize)

	return dbmd.NewDoublyEncryptedQueryWithDimentions(pk, width, height, groupSize, index)
}

// NewDoublyEncryptedQueryWithDimentions generates two encrypted point function that acts as a PIR query
// to select the row and column in the database that is viewed as a width x height grid
func (dbmd *DBMetadata) NewDoublyEncryptedQueryWithDimentions(pk *paillier.PublicKey, width, height, groupSize, index int) *DoublyEncryptedQuery {

	rowIndex, colIndex := dbmd.IndexToCoordinates(index, width, height)
	colIndex = int(colIndex / groupSize)

	if index == -1 {
		rowIndex = -1
		colIndex = -1
	}

	row := make([]*paillier.Ciphertext, height)
	for i := 0; i < height; i++ {
		if i == rowIndex {
			row[i] = pk.EncryptOne()
		} else {
			row[i] = pk.EncryptZero()
		}
	}

	groupedWidth := width / groupSize

	col := make([]*paillier.Ciphertext, groupedWidth)
	for i := 0; i < groupedWidth; i++ {
		if i == colIndex {
			col[i] = pk.EncryptOneAtLevel(paillier.EncLevelTwo)
		} else {
			col[i] = pk.EncryptZeroAtLevel(paillier.EncLevelTwo)
		}
	}

	rowQuery := &EncryptedQuery{
		Pk:        pk,
		EBits:     row,
		GroupSize: groupSize,
		DBWidth:   width,
		DBHeight:  height,
	}

	colQuery := &EncryptedQuery{
		Pk:        pk,
		EBits:     col,
		GroupSize: groupSize,
		DBWidth:   width,
		DBHeight:  1,
	}

	return &DoublyEncryptedQuery{
		Row: rowQuery,
		Col: colQuery,
	}
}

// NewAuthenticatedQuery generates an authenticated PIR query that can be verified by the server
func (dbmd *DBMetadata) NewAuthenticatedQuery(
	sk *paillier.SecretKey,
	groupSize, index int,
	authKey *Slot) (*AuthenticatedEncryptedQuery, *AuthQueryPrivateState) {

	pk := &sk.PublicKey

	queryReal := dbmd.NewDoublyEncryptedQuery(pk, groupSize, index)
	queryFake := dbmd.NewDoublyEncryptedQuery(pk, groupSize, -1)

	// TODO: have a better way of converting authKey to an encryptable type
	// since it *has* to match the format used when processing queries
	realToken := pk.Encrypt(new(gmp.Int).SetBytes(authKey.Data))
	fakeToken := pk.EncryptZero()

	var query0 *DoublyEncryptedQuery
	var query1 *DoublyEncryptedQuery
	var token0 *paillier.Ciphertext
	var token1 *paillier.Ciphertext

	bit := rand.Intn(2)
	if bit == 0 {
		query0 = queryReal
		token0 = realToken
		query1 = queryFake
		token1 = fakeToken
	} else {
		query0 = queryFake
		token0 = fakeToken
		query1 = queryReal
		token1 = realToken
	}

	authTokenComm0 := Commit(token0.C)
	authTokenComm1 := Commit(token1.C)

	authQuery := &AuthenticatedEncryptedQuery{
		Query0:         query0,
		Query1:         query1,
		AuthTokenComm0: authTokenComm0,
		AuthTokenComm1: authTokenComm1,
	}

	state := &AuthQueryPrivateState{
		Sk:         sk,
		Bit:        bit,
		AuthToken0: token0,
		AuthToken1: token1,
	}

	return authQuery, state
}

// Recover combines shares of slots to recover the data
func Recover(resShares []*SecretSharedQueryResult) []*Slot {

	numSlots := len(resShares[0].Shares)
	res := make([]*Slot, numSlots)

	// init the slots with the correct size
	for i := 0; i < numSlots; i++ {
		res[i] = &Slot{
			Data: make([]byte, resShares[0].SlotBytes),
		}
	}

	for i := 0; i < len(resShares); i++ {
		for j := 0; j < numSlots; j++ {
			XorSlots(res[j], resShares[i].Shares[j])
		}
	}

	return res
}

// RecoverEncrypted decryptes the encrypted slot and returns slot
func RecoverEncrypted(res *EncryptedQueryResult, sk *paillier.SecretKey) []*Slot {

	slots := make([]*Slot, len(res.Slots))

	// iterate over all the encrypted slots
	for i, eslot := range res.Slots {
		arr := make([]*gmp.Int, len(eslot.Cts))
		for j, ct := range eslot.Cts {
			arr[j] = sk.Decrypt(ct)
		}

		slots[i] = NewSlotFromGmpIntArray(arr, res.SlotBytes, res.NumBytesPerCiphertext)
	}

	return slots
}

// RecoverDoublyEncrypted decryptes the encrypted slot and returns slot
func RecoverDoublyEncrypted(res *DoublyEncryptedQueryResult, sk *paillier.SecretKey) []*Slot {

	slots := make([]*Slot, len(res.Slots))

	for i, slot := range res.Slots {
		arr := make([]*gmp.Int, len(slot.Cts))
		for j, c := range slot.Cts {
			arr[j] = sk.NestedDecrypt(c)
		}

		slot := NewSlotFromGmpIntArray(arr, res.SlotBytes, res.NumBytesPerCiphertext)

		slots[i] = slot
	}

	return slots
}
