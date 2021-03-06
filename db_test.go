package pir

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ncw/gmp"
	"github.com/sachaservan/paillier"
)

func setup() {
	rand.Seed(time.Now().Unix())
}

// run with 'go test -v -run TestSharedQuery' to see log outputs.
func TestSharedQuery(t *testing.T) {
	setup()

	db := GenerateRandomDB(TestDBSize, SlotBytes)

	for groupSize := MinGroupSize; groupSize < MaxGroupSize; groupSize++ {

		dimWidth := groupSize
		dimHeight := int(math.Ceil(float64(TestDBSize / dimWidth)))

		for i := 0; i < NumQueries; i++ {
			qIndex := rand.Intn(dimHeight)
			shares := db.NewIndexQueryShares(qIndex, groupSize, 2)

			resA, err := db.PrivateSecretSharedQuery(shares[0], NumProcsForQuery)
			if err != nil {
				t.Fatalf("%v", err)
			}

			resB, err := db.PrivateSecretSharedQuery(shares[1], NumProcsForQuery)
			if err != nil {
				t.Fatalf("%v", err)
			}

			resultShares := [...]*SecretSharedQueryResult{resA, resB}
			res := Recover(resultShares[:])

			for j := 0; j < dimWidth; j++ {

				index := int(qIndex)*dimWidth + j
				if index >= db.DBSize {
					break
				}

				if !db.Slots[index].Equal(res[j]) {
					t.Fatalf(
						"Query result is incorrect. %v != %v\n",
						db.Slots[index],
						res[j],
					)
				}

				t.Logf("Slot %v, is %v\n", j, res[j])
			}
		}
	}
}

// run with 'go test -v -run TestEncryptedQuery' to see log outputs.
func TestEncryptedQuery(t *testing.T) {
	setup()

	sk, pk := paillier.KeyGen(128)

	for slotBytes := 1; slotBytes < SlotBytes; slotBytes += SlotBytesStep {
		db := GenerateRandomDB(TestDBSize, SlotBytes)

		for groupSize := MinGroupSize; groupSize < MaxGroupSize; groupSize++ {

			dimWidth, dimHeight := db.GetDimentionsForDatabase(TestDBHeight, groupSize)

			for i := 0; i < NumQueries; i++ {
				qIndex := rand.Intn(dimHeight)

				query := db.NewEncryptedQuery(pk, groupSize, qIndex)

				response, err := db.PrivateEncryptedQuery(query, NumProcsForQuery)
				if err != nil {
					t.Fatalf("%v", err)
				}

				res := RecoverEncrypted(response, sk)

				if len(res)%groupSize != 0 {
					t.Fatalf("Response size is not a multiple of DBGroupSize")
				}

				for j := 0; j < dimWidth; j++ {

					index := qIndex*dimWidth + j
					if index >= db.DBSize {
						break
					}

					if !db.Slots[index].Equal(res[j]) {
						t.Fatalf(
							"Query result is incorrect. %v != %v\n",
							db.Slots[index],
							res[j],
						)
					}
				}
			}
		}
	}
}

func TestEncryptedNullQuery(t *testing.T) {
	setup()

	sk, pk := paillier.KeyGen(128)

	for slotBytes := 1; slotBytes < SlotBytes; slotBytes += SlotBytesStep {
		db := GenerateRandomDB(TestDBSize, SlotBytes)

		for groupSize := MinGroupSize; groupSize < MaxGroupSize; groupSize++ {

			dimWidth, _ := db.GetDimentionsForDatabase(TestDBHeight, groupSize)

			for i := 0; i < NumQueries; i++ {
				qIndex := -1

				query := db.NewEncryptedQuery(pk, groupSize, qIndex)

				response, err := db.PrivateEncryptedQuery(query, NumProcsForQuery)
				if err != nil {
					t.Fatalf("%v", err)
				}

				res := RecoverEncrypted(response, sk)

				if len(res)%groupSize != 0 {
					t.Fatalf("Response size is not a multiple of DBGroupSize")
				}

				emptySlot := NewEmptySlot(len(res[0].Data))
				for j := 0; j < dimWidth; j++ {

					if !emptySlot.Equal(res[j]) {
						t.Fatalf(
							"Null query incorrect for group size %v. %v != %v\n",
							groupSize,
							emptySlot,
							res[j],
						)
					}
				}
			}
		}
	}
}

func TestDoublyEncryptedNullQuery(t *testing.T) {
	setup()

	sk, pk := paillier.KeyGen(126)

	for slotBytes := 1; slotBytes < SlotBytes; slotBytes += SlotBytesStep {
		db := GenerateRandomDB(TestDBSize, SlotBytes)

		for groupSize := MinGroupSize; groupSize < MaxGroupSize; groupSize++ {

			for i := 0; i < NumQueries; i++ {

				query := db.NewDoublyEncryptedNullQuery(pk, groupSize)
				response, err := db.PrivateDoublyEncryptedQuery(query, NumProcsForQuery)
				if err != nil {
					t.Fatalf("%v", err)
				}

				res := RecoverDoublyEncrypted(response, sk)
				emptySlot := NewEmptySlot(len(res[0].Data))

				for col := 0; col < groupSize; col++ {

					if !emptySlot.Equal(res[col]) {
						t.Fatalf(
							"Null query incorrect. %v != %v\n",
							emptySlot,
							res[col],
						)
					}
				}
			}
		}
	}
}

// run with 'go test -v -run TestDoublyEncryptedQuery' to see log outputs.
func TestDoublyEncryptedQuery(t *testing.T) {
	setup()

	sk, pk := paillier.KeyGen(128)

	for slotBytes := 1; slotBytes < SlotBytes; slotBytes += SlotBytesStep {
		db := GenerateRandomDB(TestDBSize, SlotBytes)

		for groupSize := MinGroupSize; groupSize < MaxGroupSize; groupSize++ {

			dimWidth, dimHeight := db.GetDimentionsForDatabase(TestDBHeight, groupSize)

			// make sure the database width and height are not ridiculous
			// (allow for up to 1 extra row)
			if dimWidth*dimHeight > db.DBSize+dimWidth {
				t.Fatalf(
					"Dimensions are incorrect. width = %v  heigh = %v group size = %v  (%v > %v)\n",
					dimWidth,
					dimHeight,
					groupSize,
					dimWidth*dimHeight,
					db.DBSize,
				)
			}

			for i := 0; i < NumQueries; i++ {

				// select a random group
				qIndex := int(rand.Intn(dimWidth*dimHeight) / groupSize)

				query := db.NewDoublyEncryptedQuery(pk, groupSize, qIndex)

				if len(query.Col.EBits) > (dimWidth / groupSize) {
					t.Fatalf(
						"Query consists of %v encrypted bits for a db width of %v\n",
						len(query.Col.EBits),
						(query.Col.DBWidth / groupSize),
					)
				}

				response, err := db.PrivateDoublyEncryptedQuery(query, NumProcsForQuery)
				if err != nil {
					t.Fatalf("%v", err)
				}

				res := RecoverDoublyEncrypted(response, sk)

				rowIndex, colIndex := db.IndexToCoordinates(qIndex, dimWidth, dimHeight)
				colIndex = int(colIndex / groupSize)

				for j := 0; j < groupSize; j++ {

					index := rowIndex*dimWidth + colIndex*groupSize + j
					if index >= db.DBSize {
						break
					}

					if !db.Slots[index].Equal(res[j]) {
						t.Fatalf(
							"Query result is incorrect. %v != %v\n",
							db.Slots[index],
							res[j],
						)
					}
				}
			}
		}
	}
}

func BenchmarkBuildDB(b *testing.B) {
	setup()

	// benchmark index build time
	for i := 0; i < b.N; i++ {
		GenerateRandomDB(BenchmarkDBSize, SlotBytes)
	}
}

func BenchmarkQuerySecretShares(b *testing.B) {
	setup()

	db := GenerateRandomDB(BenchmarkDBSize, SlotBytes)
	queryA := db.NewIndexQueryShares(0, 1, 2)[0]

	b.ResetTimer()

	// benchmark index build time
	for i := 0; i < b.N; i++ {
		_, err := db.PrivateSecretSharedQuery(queryA, NumProcsForQuery)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkQuerySecretSharesSingleThread(b *testing.B) {
	setup()

	db := GenerateEmptyDB(BenchmarkDBSize, SlotBytes)
	queryA := db.NewIndexQueryShares(0, 1, 2)[0]

	b.ResetTimer()

	// benchmark index build time
	for i := 0; i < b.N; i++ {
		_, err := db.PrivateSecretSharedQuery(queryA, 1)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkQuerySecretSharesSingle8Thread(b *testing.B) {
	setup()

	db := GenerateEmptyDB(BenchmarkDBSize, SlotBytes)
	queryA := db.NewIndexQueryShares(0, 1, 2)[0]

	b.ResetTimer()

	// benchmark index build time
	for i := 0; i < b.N; i++ {
		_, err := db.PrivateSecretSharedQuery(queryA, 8)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkGenEncryptedQuery(b *testing.B) {
	setup()

	_, pk := paillier.KeyGen(1024)
	db := GenerateRandomDB(BenchmarkDBSize, SlotBytes)

	b.ResetTimer()

	// benchmark index build time
	for i := 0; i < b.N; i++ {
		db.NewEncryptedQuery(pk, 1, 0)
	}
}

func BenchmarkGenDoublyEncryptedQuery(b *testing.B) {
	setup()

	_, pk := paillier.KeyGen(1024)
	db := GenerateRandomDB(BenchmarkDBSize, SlotBytes)

	b.ResetTimer()

	// benchmark index build time
	for i := 0; i < b.N; i++ {
		db.NewDoublyEncryptedNullQuery(pk, 1)
	}
}

func BenchmarkEncryptedQueryAHESingleThread(b *testing.B) {
	setup()

	_, pk := paillier.KeyGen(1024)
	db := GenerateEmptyDB(BenchmarkDBSize, SlotBytes)
	query := db.NewEncryptedQuery(pk, 1, 0)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.PrivateEncryptedQuery(query, 1)

		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkEncryptedQueryAHE8Thread(b *testing.B) {
	setup()

	_, pk := paillier.KeyGen(1024)
	db := GenerateEmptyDB(BenchmarkDBSize, SlotBytes)
	query := db.NewEncryptedQuery(pk, 1, 0)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.PrivateEncryptedQuery(query, 8)

		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRecursiveEncryptedQueryAHESingleThread(b *testing.B) {
	setup()

	_, pk := paillier.KeyGen(1024)
	db := GenerateRandomDB(BenchmarkDBSize, SlotBytes)
	query := fakeDoublyEncryptedQuery(pk, db.DBSize)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.PrivateDoublyEncryptedQuery(query, 1)

		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRecursiveEncryptedQueryAHE8Thread(b *testing.B) {
	setup()

	_, pk := paillier.KeyGen(1024)
	db := GenerateEmptyDB(BenchmarkDBSize, SlotBytes)
	query := fakeDoublyEncryptedQuery(pk, db.DBSize)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.PrivateDoublyEncryptedQuery(query, 2)

		if err != nil {
			panic(err)
		}
	}
}

// generates a "fake" PIR query to avoid costly randomness computation (useful for benchmarking query processing)
func fakeDoublyEncryptedQuery(pk *paillier.PublicKey, dbSize int) *DoublyEncryptedQuery {
	zero := gmp.NewInt(0)
	one := gmp.NewInt(1)
	rand := pk.EncryptZero().C // hack to get random number

	// compute sqrt dimentions
	height := int(math.Ceil(math.Sqrt(float64(dbSize))))
	width := height
	rowIndex := 0
	colIndex := 0

	row := make([]*paillier.Ciphertext, height)
	for i := 0; i < height; i++ {
		if i == rowIndex {
			row[i] = pk.EncryptWithR(one, rand)
		} else {
			row[i] = pk.EncryptWithR(zero, rand)
		}
	}

	col := make([]*paillier.Ciphertext, width)
	for i := 0; i < width; i++ {
		if i == colIndex {
			col[i] = pk.EncryptWithRAtLevel(one, rand, paillier.EncLevelTwo)
		} else {
			col[i] = pk.EncryptWithRAtLevel(zero, rand, paillier.EncLevelTwo)
		}
	}

	rowQuery := &EncryptedQuery{
		Pk:        pk,
		EBits:     row,
		GroupSize: 1,
		DBWidth:   width * 1,
		DBHeight:  height,
	}

	colQuery := &EncryptedQuery{
		Pk:        pk,
		EBits:     col,
		GroupSize: 1,
		DBWidth:   width,
		DBHeight:  height,
	}

	return &DoublyEncryptedQuery{
		Row: rowQuery,
		Col: colQuery,
	}
}
