# Quantum Permutation Pad
![image](https://github.com/xtaci/qpp/assets/2346725/7a05cc4b-b207-437e-a015-57918906b738)

[![GoDoc][1]][2] [![Go Report Card][3]][4]

[1]: https://godoc.org/github.com/xtaci/qpp?status.svg
[2]: https://pkg.go.dev/github.com/xtaci/qpp
[3]: https://goreportcard.com/badge/github.com/xtaci/qpp
[4]: https://goreportcard.com/report/github.com/xtaci/qpp

The [Quantum Permutation Pad](https://link.springer.com/content/pdf/10.1140/epjqt/s40507-023-00164-3.pdf) (QPP) is a cryptographic protocol designed to leverage the principles of quantum mechanics for secure communication. While the exact details of the QPP can vary based on the specific implementation and the theoretical model, the general concept involves using quantum properties such as superposition and entanglement to enhance the security of data transmission. Here’s an overview of the QPP and its relationship to quantum mechanics and cryptography:

## Key Concepts of Quantum Permutation Pad

1. **Quantum Mechanics Principles**: QPP relies on fundamental quantum mechanics principles, particularly superposition (the ability of quantum bits to be in multiple states simultaneously) and entanglement (the correlation between quantum bits regardless of distance).

2. **Quantum Bits (Qubits)**: Instead of classical bits (which are either 0 or 1), QPP uses qubits, which can be in a state of 0, 1, or any quantum superposition of these states.

3. **Permutation Operations**: Permutations in the context of QPP refer to rearranging the order of computational base. These permutations can be thought of as quantum gates that alter the qubit states in a manner that is hard to predict without the correct key.

## Functionality of Quantum Permutation Pad

1. **Key Generation**: The QPP protocol involves generating a key based on quantum states. This key can be a set of quantum gates or permutation operations that will be applied to the qubits.

2. **Encryption**:
   - **Prepare Qubits**: The sender prepares a set of qubits in a known quantum state.
   - **Apply Permutations**: Using the generated key, the sender applies a series of permutation operations to the qubits. These permutations act as the encryption process, transforming the quantum state into an encrypted form.

3. **Transmission**: The encrypted quantum state (the qubits after permutation) is transmitted to the receiver.

4. **Decryption**:
   - **Reverse Permutations**: The receiver, who has the same key, applies the inverse of the permutation operations to the received qubits. This step decrypts the quantum state, returning it to its original form.
   - **Measurement**: The receiver then measures the qubits to obtain the classical data.

## Security Aspects

- **Quantum No-Cloning Theorem**: One of the fundamental principles that enhance the security of QPP is the no-cloning theorem, which states that it is impossible to create an identical copy of an arbitrary unknown quantum state. This property prevents eavesdroppers from copying the qubits during transmission.
- **Quantum Key Distribution (QKD)**: QPP can be integrated with QKD protocols like BB84 to securely distribute the key used for the permutation operations. QKD ensures that any eavesdropping attempts can be detected.
- **Unpredictability of Quantum States**: The inherent unpredictability of quantum measurements adds an extra layer of security, making it extremely difficult for an attacker to gain useful information without the correct key.

## Applications and Benefits

- **High Security**: QPP offers higher security levels compared to classical cryptographic methods, leveraging the unique properties of quantum mechanics.
- **Future-Proof**: As quantum computers become more powerful, classical cryptographic schemes (like RSA and ECC) are at risk. QPP provides a quantum-resistant alternative.
- **Secure Communication**: Useful for secure communications in quantum networks and for safeguarding highly sensitive data.

## Security design in this implementatoin
The overall security is equivalent to **1680-bit** symmetric encryption, with each **BYTE** possessing a cryptographic strength of 256 bits.

The number of permutation matrices in an 8-qubit system is determined based on the provided seed and is selected randomly.
<img width="1108" alt="image" src="https://github.com/xtaci/qpp/assets/2346725/21686542-2e94-44e0-8607-557986b4ac7b">

Try directly from https://github.com/xtaci/kcptun/releases with the ```-QPP``` option enabled.

## Performance
![image](https://github.com/user-attachments/assets/4061d4a9-e7fa-43f5-89ef-f6ef6c00a2e7)


## Usage
```golang
Internal PRNG:

func main() {
    seed := make([]byte, 32)
    io.ReadFull(rand.Reader, seed)

    qpp := NewQPP(seed, 1024)

    msg := make([]byte, 65536)
    io.ReadFull(rand.Reader, msg)

    qpp.Encrypt(msg)
    qpp.Decrypt(msg)
}
```

```golang
External PRNG:

func main() {
    seed := make([]byte, 32)
    io.ReadFull(rand.Reader, seed)

    qpp := NewQPP(seed, 1024)

    msg := make([]byte, 65536)
    io.ReadFull(rand.Reader, msg)

    rand_enc := qpp.CreatePRNG(seed)
    rand_dec := qpp.CreatePRNG(seed)

    qpp.EncryptWithPRNG(msg, rand_enc)
    qpp.DecryptWithPRNG(msg, rand_dec)
}
```



## Conclusion

The Quantum Permutation Pad is a promising approach in the field of quantum cryptography, utilizing quantum mechanical properties to achieve secure communication. By applying quantum permutations to encrypt and decrypt data, QPP ensures high security and leverages the unique capabilities of quantum technology. As research and technology in quantum computing and quantum communication advance, protocols like QPP will play a crucial role in the next generation of secure communication systems.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any improvements, bug fixes, or additional features.

## License

This project is licensed under the GPLv3 License. See the [LICENSE](LICENSE) file for details.

## References

For more detailed information, please refer to the [research paper](https://link.springer.com/content/pdf/10.1140/epjqt/s40507-023-00164-3.pdf).

## Acknowledgments

Special thanks to the authors of the research paper for their groundbreaking work on Quantum Permutation Pad.
