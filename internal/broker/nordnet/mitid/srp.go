package mitid

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// SRP implements the SRP-6a protocol with MitID-specific modifications.
type SRP struct {
	// The 2048-bit prime modulus N
	N *big.Int
	// Generator g = 2
	g *big.Int

	// Client ephemeral private key (a)
	a *big.Int
	// Client ephemeral public key (A = g^a mod N)
	A *big.Int
	// Server ephemeral public key (B)
	B *big.Int

	// Hashed password (x)
	hashedPassword *big.Int

	// Session key bytes (K = SHA256(S))
	KBits []byte

	// M1 proof hex string
	M1Hex string
}

// srpN is the 2048-bit prime used by MitID.
var srpN *big.Int

func init() {
	srpN = new(big.Int)
	srpN.SetString("4983313092069490398852700692508795473567251422586244806694940877242664573189903192937797446992068818099986958054998012331720869136296780936009508700487789962429161515853541556719593346959929531150706457338429058926505817847524855862259333438239756474464759974189984231409170758360686392625635632084395639143229889862041528635906990913087245817959460948345336333086784608823084788906689865566621015175424691535711520273786261989851360868669067101108956159530739641990220546209432953829448997561743719584980402874346226230488627145977608389858706391858138200618631385210304429902847702141587470513336905449351327122086464725143970313054358650488241167131544692349123381333204515637608656643608393788598011108539679620836313915590459891513992208387515629240292926570894321165482608544030173975452781623791805196546326996790536207359143527182077625412731080411108775183565594553871817639221414953634530830290393130518228654795859", 10)
}

// NewSRP creates a new SRP instance.
func NewSRP() *SRP {
	return &SRP{
		N: srpN,
		g: big.NewInt(2),
	}
}

// Stage1 generates the client ephemeral key pair.
// Returns A as hex string.
func (s *SRP) Stage1() (string, error) {
	// Generate random 256-bit private key
	aBytes := make([]byte, 32)
	if _, err := rand.Read(aBytes); err != nil {
		return "", fmt.Errorf("generating random a: %w", err)
	}

	s.a = new(big.Int).SetBytes(aBytes)

	// Ensure a is positive and within range
	if s.a.Sign() < 0 {
		s.a.Add(s.a, s.N)
	}

	// Compute A = g^a mod N
	s.A = new(big.Int).Exp(s.g, s.a, s.N)

	return fmt.Sprintf("%x", s.A), nil
}

// computeLittleK computes k = SHA256(str(N) || PAD(g))
// CRITICAL: MitID uses decimal string representation of N!
func (s *SRP) computeLittleK() *big.Int {
	nBytes := s.N.Bytes()
	gBytes := s.g.Bytes()

	// Pad g to same length as N
	paddedG := make([]byte, len(nBytes))
	copy(paddedG[len(nBytes)-len(gBytes):], gBytes)

	// Hash: str(N) as decimal string + padded g bytes
	h := sha256.New()
	h.Write([]byte(s.N.String())) // Decimal string representation
	h.Write(paddedG)

	digest := h.Sum(nil)
	return new(big.Int).SetBytes(digest)
}

// computeU computes u = SHA256(PAD(A) || PAD(B)) mod N
func (s *SRP) computeU() *big.Int {
	nLen := len(s.N.Bytes())

	// Pad A to N length
	aBytes := s.A.Bytes()
	paddedA := make([]byte, nLen)
	copy(paddedA[nLen-len(aBytes):], aBytes)

	// Pad B to N length
	bBytes := s.B.Bytes()
	paddedB := make([]byte, nLen)
	copy(paddedB[nLen-len(bBytes):], bBytes)

	h := sha256.New()
	h.Write(paddedA)
	h.Write(paddedB)

	digest := h.Sum(nil)
	u := new(big.Int).SetBytes(digest)
	return u.Mod(u, s.N)
}

// computeSessionKey computes the session key S.
// S = (B - k*g^x)^(a + u*x) mod N
func (s *SRP) computeSessionKey() *big.Int {
	u := s.computeU()
	k := s.computeLittleK()

	// g^x mod N
	gx := new(big.Int).Exp(s.g, s.hashedPassword, s.N)

	// k * g^x mod N
	kgx := new(big.Int).Mul(k, gx)
	kgx.Mod(kgx, s.N)

	// B - k*g^x mod N
	base := new(big.Int).Sub(s.B, kgx)
	base.Mod(base, s.N)

	// a + u*x
	ux := new(big.Int).Mul(u, s.hashedPassword)
	exp := new(big.Int).Add(s.a, ux)

	// Ensure positive exponent
	if exp.Sign() < 0 {
		exp.Add(exp, s.N)
	}

	// S = base^exp mod N
	return new(big.Int).Exp(base, exp, s.N)
}

// computeM1 computes the client proof M1.
// M1 = SHA256(SHA256(N)^SHA256(g) || SHA256(sessionId) || srpSalt || A || B || hex(K))
func (s *SRP) computeM1(authSessionID, srpSalt string) string {
	// SHA256(N) using decimal string
	hN := sha256.Sum256([]byte(s.N.String()))
	nHash := new(big.Int).SetBytes(hN[:])

	// SHA256(g) using decimal string
	hG := sha256.Sum256([]byte(s.g.String()))
	gHash := new(big.Int).SetBytes(hG[:])

	// XOR of hashes
	xorResult := new(big.Int).Xor(nHash, gHash)

	// SHA256(sessionId)
	hI := sha256.Sum256([]byte(authSessionID))
	iHex := hex.EncodeToString(hI[:])

	// Build M1 input: str(xor) + I_hex + srpSalt + str(A) + str(B) + hex(K)
	m1Input := fmt.Sprintf("%s%s%s%s%s%s",
		xorResult.String(),
		iHex,
		srpSalt,
		s.A.String(),
		s.B.String(),
		hex.EncodeToString(s.KBits),
	)

	h := sha256.Sum256([]byte(m1Input))
	return hex.EncodeToString(h[:])
}

// Stage3 processes the server's challenge and computes the client proof M1.
// Parameters:
//   - srpSalt: The SRP salt from server (hex string)
//   - randomB: The server's ephemeral public key B (hex string)
//   - password: The password (already hashed for APP method, or PBKDF2-derived for PASSWORD)
//   - authSessionID: The authentication session ID
//
// Returns M1 proof as hex string.
func (s *SRP) Stage3(srpSalt, randomB, password, authSessionID string) (string, error) {
	// Parse B from hex
	s.B = new(big.Int)
	if _, ok := s.B.SetString(randomB, 16); !ok {
		return "", fmt.Errorf("invalid randomB hex")
	}

	// Safety check: B != 0 and B % N != 0
	if s.B.Sign() == 0 {
		return "", fmt.Errorf("randomB is zero")
	}
	if new(big.Int).Mod(s.B, s.N).Sign() == 0 {
		return "", fmt.Errorf("randomB mod N is zero")
	}

	// Hash password: x = SHA256(srpSalt || password)
	h := sha256.New()
	h.Write([]byte(srpSalt + password))
	passwordHash := h.Sum(nil)
	s.hashedPassword = new(big.Int).SetBytes(passwordHash)

	// Compute session key S
	sessionKeyS := s.computeSessionKey()

	// K = SHA256(str(S)) - using decimal string representation
	kHash := sha256.Sum256([]byte(sessionKeyS.String()))
	s.KBits = kHash[:]

	// Compute M1
	s.M1Hex = s.computeM1(authSessionID, srpSalt)

	return s.M1Hex, nil
}

// Stage5 verifies the server's proof M2.
// M2 = SHA256(str(A) || str(M1_int) || hex(K))
func (s *SRP) Stage5(m2Hex string) bool {
	// Parse M1 as big int
	m1Int := new(big.Int)
	m1Int.SetString(s.M1Hex, 16)

	// Build verification input
	input := fmt.Sprintf("%s%s%s",
		s.A.String(),
		m1Int.String(),
		hex.EncodeToString(s.KBits),
	)

	h := sha256.Sum256([]byte(input))
	expectedM2 := hex.EncodeToString(h[:])

	return expectedM2 == m2Hex
}

// AuthEnc encrypts plaintext using AES-GCM with the session key K.
// Returns base64-encoded IV || ciphertext || tag.
func (s *SRP) AuthEnc(plaintext []byte) (string, error) {
	return AESGCMEncrypt(plaintext, s.KBits)
}

// AuthDec decrypts ciphertext using AES-GCM with the session key K.
func (s *SRP) AuthDec(encryptedB64 string) ([]byte, error) {
	return AESGCMDecrypt(encryptedB64, s.KBits)
}

// AuthDecPin decrypts ciphertext using the PIN-derived key.
// pin_key = SHA256(hex(K) + "PIN")
func (s *SRP) AuthDecPin(encryptedB64 string) ([]byte, error) {
	pinKey := DerivePinKey(s.KBits)
	return AESGCMDecrypt(encryptedB64, pinKey)
}

// GetSessionKey returns the session key K bytes.
func (s *SRP) GetSessionKey() []byte {
	return s.KBits
}
