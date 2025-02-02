package prover

import (
	"io"
	"strconv"
	"worldcoin/gnark-mbu/prover/poseidon"

	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/reilabs/gnark-lean-extractor/abstractor"
)

type Proof struct {
	Proof groth16.Proof
}

type ProvingSystem struct {
	TreeDepth        uint32
	BatchSize        uint32
	ProvingKey       groth16.ProvingKey
	VerifyingKey     groth16.VerifyingKey
	ConstraintSystem constraint.ConstraintSystem
}

const emptyLeaf = 0

type bitPatternLengthError struct {
	actualLength int
}

func (e *bitPatternLengthError) Error() string {
	return "Bit pattern length was " + strconv.Itoa(e.actualLength) + " not a total number of bytes"
}

type ProofRound struct {
	Direction frontend.Variable
	Hash      frontend.Variable
	Sibling   frontend.Variable
}

func (gadget ProofRound) DefineGadget(api abstractor.API) []frontend.Variable {
	api.AssertIsBoolean(gadget.Direction)
	d1 := api.Select(gadget.Direction, gadget.Hash, gadget.Sibling)
	d2 := api.Select(gadget.Direction, gadget.Sibling, gadget.Hash)
	sum := api.Call(poseidon.Poseidon2{In1: d1, In2: d2})[0]
	return []frontend.Variable{sum}
}

type VerifyProof struct {
	Proof []frontend.Variable
	Path  []frontend.Variable
}

func (gadget VerifyProof) DefineGadget(api abstractor.API) []frontend.Variable {
	sum := gadget.Proof[0]
	for i := 1; i < len(gadget.Proof); i++ {
		sum = api.Call(ProofRound{Direction: gadget.Path[i-1], Hash: gadget.Proof[i], Sibling: sum})[0]
	}
	return []frontend.Variable{sum}
}

// ReducedModRCheck Checks a little-endian array of bits asserting that it represents a number that
// is less than the field modulus R.
type ReducedModRCheck struct {
	Input []frontend.Variable
}

func (r *ReducedModRCheck) DefineGadget(api abstractor.API) []frontend.Variable {
	field := api.Compiler().Field()
	if len(r.Input) < field.BitLen() {
		// input is shorter than the field, so it's definitely reduced
		return []frontend.Variable{}
	}
	var failed frontend.Variable = 0    // we already know number is > R
	var succeeded frontend.Variable = 0 // we already know number is < R
	for i := len(r.Input) - 1; i >= 0; i-- {
		api.AssertIsBoolean(r.Input[i])
		if field.Bit(i) == 0 {
			// if number is not already < R, a 1 in this position means it's > R
			failed = api.Select(succeeded, 0, api.Or(r.Input[i], failed))
		} else {
			bitNeg := api.Sub(1, r.Input[i])
			// if number isn't already > R, a 0 in this position means it's < R
			succeeded = api.Select(failed, 0, api.Or(bitNeg, succeeded))
		}
	}
	api.AssertIsEqual(succeeded, 1)
	return []frontend.Variable{}
}

// SwapBitArrayEndianness Swaps the endianness of the bit pattern in bits,
// returning the result in newBits.
//
// It does not introduce any new circuit constraints as it simply moves the
// variables (that will later be instantiated to bits) around in the slice to
// change the byte ordering. It has been verified to be a constraint-neutral
// operation, so please maintain this invariant when modifying it.
//
// Raises a bitPatternLengthError if the length of bits is not a multiple of a
// number of bytes.
func SwapBitArrayEndianness(bits []frontend.Variable) (newBits []frontend.Variable, err error) {
	bitPatternLength := len(bits)

	if bitPatternLength%8 != 0 {
		return nil, &bitPatternLengthError{bitPatternLength}
	}

	for i := bitPatternLength - 8; i >= 0; i -= 8 {
		currentBytes := bits[i : i+8]
		newBits = append(newBits, currentBytes...)
	}

	if bitPatternLength != len(newBits) {
		return nil, &bitPatternLengthError{len(newBits)}
	}

	return newBits, nil
}

// ToReducedBinaryBigEndian converts the provided variable to the corresponding bit
// pattern using big-endian byte ordering. It also makes sure to pick the smallest
// binary representation (i.e. one that is reduced modulo scalar field order).
//
// Raises a bitPatternLengthError if the number of bits in variable is not a
// whole number of bytes.
func ToReducedBinaryBigEndian(variable frontend.Variable, size int, api frontend.API) (bitsBigEndian []frontend.Variable, err error) {
	bitsLittleEndian := api.ToBinary(variable, size)
	abstractor.CallGadget(api, &ReducedModRCheck{Input: bitsLittleEndian})
	return SwapBitArrayEndianness(bitsLittleEndian)
}

// FromBinaryBigEndian converts the provided bit pattern that uses big-endian
// byte ordering to a variable that uses little-endian byte ordering.
//
// Raises a bitPatternLengthError if the number of bits in `bitsBigEndian` is not
// a whole number of bytes.
func FromBinaryBigEndian(bitsBigEndian []frontend.Variable, api frontend.API) (variable frontend.Variable, err error) {
	bitsLittleEndian, err := SwapBitArrayEndianness(bitsBigEndian)
	if err != nil {
		return nil, err
	}

	return api.FromBinary(bitsLittleEndian...), nil
}

func toBytesLE(b []byte) []byte {
	for i := 0; i < len(b)/2; i++ {
		b[i], b[len(b)-i-1] = b[len(b)-i-1], b[i]
	}
	return b
}

func (ps *ProvingSystem) ExportSolidity(writer io.Writer) error {
	return ps.VerifyingKey.ExportSolidity(writer)
}
