package main

import (
	"os"
	"worldcoin/gnark-mbu/prover"
)

func main() {
	system, err := prover.SetupInsertion(16, 3)

	if err != nil {
		println("error when setting up insertion", err)
	}

	file, err := os.Create("Verifier.sol")
	if err != nil {
		println("Failed to create file:", err)
		return
	}
	defer file.Close()

	err = system.VerifyingKey.ExportSolidity(file)
	if err != nil {
		println("Failed to export solidity:", err)
		return
	}

}
