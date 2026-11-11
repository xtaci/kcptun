<p align="center">
  <img src="https://cdn.rawgit.com/awnumar/memguard/master/logo.svg" height="140" />
  <h3 align="center">MemGuard</h3>
  <p align="center">Software enclave for storage of sensitive information in memory.</p>
  <p align="center">
    <a href="https://github.com/awnumar/memguard/actions/workflows/ci.yml"><img src="https://github.com/awnumar/memguard/actions/workflows/ci.yml/badge.svg?branch=master"></a>
    <a href="https://pkg.go.dev/github.com/awnumar/memguard?tab=doc"><img src="https://godoc.org/github.com/awnumar/memguard?status.svg"></a>
  </p>
</p>

---

This package attempts to reduce the likelihood of sensitive data being exposed when in memory. It aims to support all major operating systems and is written in pure Go.

## Features

* Sensitive data is encrypted and authenticated in memory with XSalsa20Poly1305. The [scheme](https://spacetime.dev/encrypting-secrets-in-memory) used also [defends against cold-boot attacks](https://spacetime.dev/memory-retention-attacks).
* Memory allocation bypasses the language runtime by [using system calls](https://github.com/awnumar/memcall) to query the kernel for resources directly. This avoids interference from the garbage-collector.
* Buffers that store plaintext data are fortified with guard pages and canary values to detect spurious accesses and overflows.
* Effort is taken to prevent sensitive data from touching the disk. This includes locking memory to prevent swapping and handling core dumps.
* Kernel-level immutability is implemented so that attempted modification of protected regions results in an access violation.
* Multiple endpoints provide session purging and safe termination capabilities as well as signal handling to prevent remnant data being left behind.
* Side-channel attacks are mitigated against by making sure that the copying and comparison of data is done in constant-time.

Some features were inspired by [libsodium](https://github.com/jedisct1/libsodium), so credits to them.

Full documentation and a complete overview of the API can be found [here](https://godoc.org/github.com/awnumar/memguard). Interesting and useful code samples can be found within the [examples](examples) subpackage.

## Installation

```
$ go get github.com/awnumar/memguard
```

API is experimental and may have unstable changes. You should pin a version. [[modules](https://github.com/golang/go/wiki/Modules)]

## Contributing

* Submitting program samples to [`./examples`](examples).
* Reporting bugs, vulnerabilities, and any difficulties in using the API.
* Writing useful security and crypto libraries that utilise memguard.
* Implementing kernel-specific/cpu-specific protections.
* Submitting performance improvements.

Issues are for reporting bugs and for discussion on proposals. Pull requests should be made against master.
