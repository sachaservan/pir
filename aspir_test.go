package pir

import (
	"math"
	"math/rand"
	"testing"

	"github.com/sachaservan/paillier"
)

// run with 'go test -v -run TestASPIR' to see log outputs.
func TestASPIR(t *testing.T) {
	secparam := StatisticalSecurityParam // statistical secuirity parameter for proof soundness
	nprocs := 1

	sk, pk := paillier.KeyGen(128)

	db := GenerateRandomDB(TestDBSize, int(secparam/4)) // get secparam in bytes

	for i := 0; i < NumQueries; i++ {

		for groupSize := MinGroupSize; groupSize < MaxGroupSize; groupSize++ {

			keydbSize := int(math.Ceil(float64(TestDBSize / groupSize)))
			keydb := GenerateRandomDB(keydbSize, int(secparam/4)) // get secparam in bytes
			qIndex := rand.Intn(keydb.DBSize)

			// generate auth token consisiting of double encryption of the key
			authKey := keydb.Slots[qIndex]
			authQuery, state := db.NewAuthenticatedQuery(sk, groupSize, qIndex, authKey)

			t.Logf("authToken0 = %v\n", sk.Decrypt(state.AuthToken0))
			t.Logf("authToken1 = %v\n", sk.Decrypt(state.AuthToken1))

			// issue challenge
			chalToken, err := AuthChalForQuery(secparam, keydb, authQuery, nprocs)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("chal0 = %v\n", sk.NestedDecrypt(chalToken.Token0))
			t.Logf("chal1 = %v\n", sk.NestedDecrypt(chalToken.Token1))

			// generate proof
			proofToken, err := AuthProve(state, chalToken)
			if err != nil {
				t.Fatal(err)
			}

			// generate proof
			ok := AuthCheck(pk, authQuery, chalToken, proofToken)
			if !ok {
				t.Fatalf("ASPIR proof failed")
			}
		}
	}
}

// run with 'go test -v -run TestSharedASPIRCompleteness' to see log outputs.
func TestSharedASPIRCompleteness(t *testing.T) {

	secparam := StatisticalSecurityParam // statistical secuirity parameter for proof soundness

	keydb := GenerateRandomDB(TestDBSize, int(secparam/4)) // get secparam in bytes

	for i := 0; i < NumQueries; i++ {
		index := rand.Intn(TestDBSize)

		// generate auth token consisiting of double encryption of the key
		authKey := keydb.Slots[index]
		authTokenShares := AuthTokenSharesForKey(authKey, 2)
		queryShares := keydb.NewIndexQueryShares(index, 1, 2)

		audits := make([]*AuditTokenShare, 2)
		audits[0], _ = GenerateAuditForSharedQuery(keydb, queryShares[0], authTokenShares[0], 1)
		audits[1], _ = GenerateAuditForSharedQuery(keydb, queryShares[1], authTokenShares[1], 1)

		// generate proof
		ok := CheckAudit(audits...)
		if !ok {
			t.Fatalf("Secret shared ASPIR proof failed")
		}

	}
}

// run with 'go test -v -run TestSharedASPIRSoundness' to see log outputs.
func TestSharedASPIRSoundness(t *testing.T) {

	secparam := StatisticalSecurityParam // statistical secuirity parameter for proof soundness

	keydb := GenerateRandomDB(TestDBSize, int(secparam/4)) // get secparam in bytes

	for i := 0; i < NumQueries; i++ {
		index := rand.Intn(TestDBSize-1) + 1

		// generate auth token consisiting of double encryption of the key
		authKey := keydb.Slots[0]
		authTokenShares := AuthTokenSharesForKey(authKey, 2)
		queryShares := keydb.NewIndexQueryShares(index, 1, 2)

		audits := make([]*AuditTokenShare, 2)
		audits[0], _ = GenerateAuditForSharedQuery(keydb, queryShares[0], authTokenShares[0], 1)
		audits[1], _ = GenerateAuditForSharedQuery(keydb, queryShares[1], authTokenShares[1], 1)

		// generate proof
		ok := CheckAudit(audits...)
		if ok {
			t.Fatalf("ASPIR proof succeeded with a false auth key")
		}

	}
}

func BenchmarkChallenge(b *testing.B) {
	secparam := StatisticalSecurityParam // statistical secuirity parameter for proof soundness

	sk, _ := paillier.KeyGen(1024)
	keydb := GenerateRandomDB(BenchmarkDBSize, int(secparam/4))

	// generate auth token consisiting of double encryption of the key
	authKey := keydb.Slots[0]
	authQuery, _ := keydb.DBMetadata.NewAuthenticatedQuery(sk, 1, 0, authKey)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := AuthChalForQuery(secparam, keydb, authQuery, 1)

		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkProve(b *testing.B) {
	secparam := StatisticalSecurityParam // statistical secuirity parameter for proof soundness

	sk, _ := paillier.KeyGen(1024)
	keydb := GenerateRandomDB(BenchmarkDBSize, int(secparam/4))

	// generate auth token consisiting of double encryption of the key
	authKey := keydb.Slots[0]
	authQuery, state := keydb.DBMetadata.NewAuthenticatedQuery(sk, 1, 0, authKey)

	// issue challenge
	chalToken, _ := AuthChalForQuery(secparam, keydb, authQuery, 1)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := AuthProve(state, chalToken)

		if err != nil {
			panic(err)
		}
	}
}
