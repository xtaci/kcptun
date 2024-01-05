# Reed-Solomon
[![Go Reference](https://pkg.go.dev/badge/github.com/klauspost/reedsolomon.svg)](https://pkg.go.dev/github.com/klauspost/reedsolomon) [![Go](https://github.com/klauspost/reedsolomon/actions/workflows/go.yml/badge.svg)](https://github.com/klauspost/reedsolomon/actions/workflows/go.yml)

Reed-Solomon Erasure Coding in Go, with speeds exceeding 1GB/s/cpu core implemented in pure Go.

This is a Go port of the [JavaReedSolomon](https://github.com/Backblaze/JavaReedSolomon) library released by 
[Backblaze](http://backblaze.com), with some additional optimizations.

For an introduction on erasure coding, see the post on the [Backblaze blog](https://www.backblaze.com/blog/reed-solomon/).

For encoding high shard counts (>256) a Leopard implementation is used.
For most platforms this performs close to the original Leopard implementation in terms of speed. 

Package home: https://github.com/klauspost/reedsolomon

Godoc: https://pkg.go.dev/github.com/klauspost/reedsolomon

# Installation
To get the package use the standard:
```bash
go get -u github.com/klauspost/reedsolomon
```

Using Go modules is recommended.

# Changes

## 2022

* [GFNI](https://github.com/klauspost/reedsolomon/pull/224) support for amd64, for up to 3x faster processing.
* [Leopard GF8](https://github.com/klauspost/reedsolomon#leopard-gf8) mode added, for faster processing of medium shard counts.
* [Leopard GF16](https://github.com/klauspost/reedsolomon#leopard-compatible-gf16) mode added, for up to 65536 shards. 
* [WithJerasureMatrix](https://pkg.go.dev/github.com/klauspost/reedsolomon?tab=doc#WithJerasureMatrix) allows constructing a [Jerasure](https://github.com/tsuraan/Jerasure) compatible matrix.

## 2021

* Use `GOAMD64=v4` to enable faster AVX2.
* Add progressive shard encoding.
* Wider AVX2 loops
* Limit concurrency on AVX2, since we are likely memory bound.
* Allow 0 parity shards.
* Allow disabling inversion cache.
* Faster AVX2 encoding.

<details>
	<summary>See older changes</summary>

## May 2020

* ARM64 optimizations, up to 2.5x faster.
* Added [WithFastOneParityMatrix](https://pkg.go.dev/github.com/klauspost/reedsolomon?tab=doc#WithFastOneParityMatrix) for faster operation with 1 parity shard.
* Much better performance when using a limited number of goroutines.
* AVX512 is now using multiple cores.
* Stream processing overhaul, big speedups in most cases.
* AVX512 optimizations

## March 6, 2019

The pure Go implementation is about 30% faster. Minor tweaks to assembler implementations.

## February 8, 2019

AVX512 accelerated version added for Intel Skylake CPUs. This can give up to a 4x speed improvement as compared to AVX2.
See [here](https://github.com/klauspost/reedsolomon#performance-on-avx512) for more details.

## December 18, 2018

Assembly code for ppc64le has been contributed, this boosts performance by about 10x on this platform.

## November 18, 2017

Added [WithAutoGoroutines](https://godoc.org/github.com/klauspost/reedsolomon#WithAutoGoroutines) which will attempt 
to calculate the optimal number of goroutines to use based on your expected shard size and detected CPU.

## October 1, 2017

* [Cauchy Matrix](https://godoc.org/github.com/klauspost/reedsolomon#WithCauchyMatrix) is now an option. 
Thanks to [templexxx](https://github.com/templexxx) for the basis of this.

* Default maximum number of [goroutines](https://godoc.org/github.com/klauspost/reedsolomon#WithMaxGoroutines) 
has been increased for better multi-core scaling.

* After several requests the Reconstruct and ReconstructData now slices of zero length but sufficient capacity to 
be used instead of allocating new memory.

## August 26, 2017

*  The [`Encoder()`](https://godoc.org/github.com/klauspost/reedsolomon#Encoder) now contains an `Update` 
function contributed by [chenzhongtao](https://github.com/chenzhongtao).

* [Frank Wessels](https://github.com/fwessels) kindly contributed ARM 64 bit assembly, 
which gives a huge performance boost on this platform.

## July 20, 2017

`ReconstructData` added to [`Encoder`](https://godoc.org/github.com/klauspost/reedsolomon#Encoder) interface. 
This can cause compatibility issues if you implement your own Encoder. A simple workaround can be added:

```Go
func (e *YourEnc) ReconstructData(shards [][]byte) error {
	return ReconstructData(shards)
}
```

You can of course also do your own implementation. 
The [`StreamEncoder`](https://godoc.org/github.com/klauspost/reedsolomon#StreamEncoder) 
handles this without modifying the interface. 
This is a good lesson on why returning interfaces is not a good design.

</details>

# Usage

This section assumes you know the basics of Reed-Solomon encoding. 
A good start is this [Backblaze blog post](https://www.backblaze.com/blog/reed-solomon/).

This package performs the calculation of the parity sets. The usage is therefore relatively simple.

First of all, you need to choose your distribution of data and parity shards. 
A 'good' distribution is very subjective, and will depend a lot on your usage scenario. 

To create an encoder with 10 data shards (where your data goes) and 3 parity shards (calculated):
```Go
    enc, err := reedsolomon.New(10, 3)
```
This encoder will work for all parity sets with this distribution of data and parity shards. 

If you will primarily be using it with one shard size it is recommended to use 
[`WithAutoGoroutines(shardSize)`](https://pkg.go.dev/github.com/klauspost/reedsolomon?tab=doc#WithAutoGoroutines)
as an additional parameter. This will attempt to calculate the optimal number of goroutines to use for the best speed.
It is not required that all shards are this size. 

Then you send and receive data that is a simple slice of byte slices; `[][]byte`. 
In the example above, the top slice must have a length of 13.

```Go
    data := make([][]byte, 13)
```
You should then fill the 10 first slices with *equally sized* data, 
and create parity shards that will be populated with parity data. In this case we create the data in memory, 
but you could for instance also use [mmap](https://github.com/edsrzf/mmap-go) to map files.

```Go
    // Create all shards, size them at 50000 each
    for i := range input {
      data[i] := make([]byte, 50000)
    }
    
    // The above allocations can also be done by the encoder:
    // data := enc.(reedsolomon.Extended).AllocAligned(50000)
    
    // Fill some data into the data shards
    for i, in := range data[:10] {
      for j:= range in {
         in[j] = byte((i+j)&0xff)
      }
    }
```

To populate the parity shards, you simply call `Encode()` with your data.
```Go
    err = enc.Encode(data)
```
The only cases where you should get an error is, if the data shards aren't of equal size. 
The last 3 shards now contain parity data. You can verify this by calling `Verify()`:

```Go
    ok, err = enc.Verify(data)
```

The final (and important) part is to be able to reconstruct missing shards. 
For this to work, you need to know which parts of your data is missing. 
The encoder *does not know which parts are invalid*, so if data corruption is a likely scenario, 
you need to implement a hash check for each shard. 

If a byte has changed in your set, and you don't know which it is, there is no way to reconstruct the data set.

To indicate missing data, you set the shard to nil before calling `Reconstruct()`:

```Go
    // Delete two data shards
    data[3] = nil
    data[7] = nil
    
    // Reconstruct the missing shards
    err := enc.Reconstruct(data)
```
The missing data and parity shards will be recreated. If more than 3 shards are missing, the reconstruction will fail.

If you are only interested in the data shards (for reading purposes) you can call `ReconstructData()`:

```Go
    // Delete two data shards
    data[3] = nil
    data[7] = nil
    
    // Reconstruct just the missing data shards
    err := enc.ReconstructData(data)
```

If you don't need all data shards you can use `ReconstructSome()`:

```Go
    // Delete two data shards
    data[3] = nil
    data[7] = nil
    
    // Reconstruct just the shard 3
    err := enc.ReconstructSome(data, []bool{false, false, false, true, false, false, false, false})
```

So to sum up reconstruction:
* The number of data/parity shards must match the numbers used for encoding.
* The order of shards must be the same as used when encoding.
* You may only supply data you know is valid.
* Invalid shards should be set to nil.

For complete examples of an encoder and decoder see the 
[examples folder](https://github.com/klauspost/reedsolomon/tree/master/examples).

# Splitting/Joining Data

You might have a large slice of data. 
To help you split this, there are some helper functions that can split and join a single byte slice.

```Go
   bigfile, _ := ioutil.Readfile("myfile.data")
   
   // Split the file
   split, err := enc.Split(bigfile)
```
This will split the file into the number of data shards set when creating the encoder and create empty parity shards. 

An important thing to note is that you have to *keep track of the exact input size*. 
If the size of the input isn't divisible by the number of data shards, extra zeros will be inserted in the last shard.

To join a data set, use the `Join()` function, which will join the shards and write it to the `io.Writer` you supply: 
```Go
   // Join a data set and write it to io.Discard.
   err = enc.Join(io.Discard, data, len(bigfile))
```

## Aligned Allocations

For AMD64 aligned inputs can make a big speed difference.

This is an example of the speed difference when inputs are unaligned/aligned:

```
BenchmarkEncode100x20x10000-32    	    7058	    172648 ns/op	6950.57 MB/s
BenchmarkEncode100x20x10000-32    	    8406	    137911 ns/op	8701.24 MB/s
```

This is mostly the case when dealing with odd-sized shards. 

To facilitate this the package provides an `AllocAligned(shards, each int) [][]byte`. 
This will allocate a number of shards, each with the size `each`.
Each shard will then be aligned to a 64 byte boundary.

Each encoder also has a `AllocAligned(each int) [][]byte` as an extended interface which will return the same, 
but with the shard count configured in the encoder.   

It is not possible to re-aligned already allocated slices, for example when using `Split`.
When it is not possible to write to aligned shards, you should not copy to them.

# Progressive encoding

It is possible to encode individual shards using EncodeIdx:

```Go
	// EncodeIdx will add parity for a single data shard.
	// Parity shards should start out as 0. The caller must zero them.
	// Data shards must be delivered exactly once. There is no check for this.
	// The parity shards will always be updated and the data shards will remain the same.
	EncodeIdx(dataShard []byte, idx int, parity [][]byte) error
```

This allows progressively encoding the parity by sending individual data shards.
There is no requirement on shards being delivered in order, 
but when sent in order it allows encoding shards one at the time,
effectively allowing the operation to be streaming. 

The result will be the same as encoding all shards at once.
There is a minor speed penalty using this method, so send 
shards at once if they are available.

## Example

```Go
func test() {
    // Create an encoder with 7 data and 3 parity slices.
    enc, _ := reedsolomon.New(7, 3)

    // This will be our output parity.
    parity := make([][]byte, 3)
    for i := range parity {
        parity[i] = make([]byte, 10000)
    }

    for i := 0; i < 7; i++ {
        // Send data shards one at the time.
        _ = enc.EncodeIdx(make([]byte, 10000), i, parity)
    }

    // parity now contains parity, as if all data was sent in one call.
}
```

# Streaming/Merging

It might seem like a limitation that all data should be in memory, 
but an important property is that *as long as the number of data/parity shards are the same, 
you can merge/split data sets*, and they will remain valid as a separate set.

```Go
    // Split the data set of 50000 elements into two of 25000
    splitA := make([][]byte, 13)
    splitB := make([][]byte, 13)
    
    // Merge into a 100000 element set
    merged := make([][]byte, 13)
    
    for i := range data {
      splitA[i] = data[i][:25000]
      splitB[i] = data[i][25000:]
      
      // Concatenate it to itself
	  merged[i] = append(make([]byte, 0, len(data[i])*2), data[i]...)
	  merged[i] = append(merged[i], data[i]...)
    }
    
    // Each part should still verify as ok.
    ok, err := enc.Verify(splitA)
    if ok && err == nil {
        log.Println("splitA ok")
    }
    
    ok, err = enc.Verify(splitB)
    if ok && err == nil {
        log.Println("splitB ok")
    }
    
    ok, err = enc.Verify(merge)
    if ok && err == nil {
        log.Println("merge ok")
    }
```

This means that if you have a data set that may not fit into memory, you can split processing into smaller blocks. 
For the best throughput, don't use too small blocks.

This also means that you can divide big input up into smaller blocks, and do reconstruction on parts of your data. 
This doesn't give the same flexibility of a higher number of data shards, but it will be much more performant.

# Streaming API

There has been added support for a streaming API, to help perform fully streaming operations, 
which enables you to do the same operations, but on streams. 
To use the stream API, use [`NewStream`](https://godoc.org/github.com/klauspost/reedsolomon#NewStream) function 
to create the encoding/decoding interfaces. 

You can use [`WithConcurrentStreams`](https://godoc.org/github.com/klauspost/reedsolomon#WithConcurrentStreams) 
to ready an interface that reads/writes concurrently from the streams.

You can specify the size of each operation using 
[`WithStreamBlockSize`](https://godoc.org/github.com/klauspost/reedsolomon#WithStreamBlockSize).
This will set the size of each read/write operation.

Input is delivered as `[]io.Reader`, output as `[]io.Writer`, and functionality corresponds to the in-memory API. 
Each stream must supply the same amount of data, similar to how each slice must be similar size with the in-memory API. 
If an error occurs in relation to a stream, 
a [`StreamReadError`](https://godoc.org/github.com/klauspost/reedsolomon#StreamReadError) 
or [`StreamWriteError`](https://godoc.org/github.com/klauspost/reedsolomon#StreamWriteError) 
will help you determine which stream was the offender.

There is no buffering or timeouts/retry specified. If you want to add that, you need to add it to the Reader/Writer.

For complete examples of a streaming encoder and decoder see the 
[examples folder](https://github.com/klauspost/reedsolomon/tree/master/examples).

GF16 (more than 256 shards) is not supported by the streaming interface. 

# Advanced Options

You can modify internal options which affects how jobs are split between and processed by goroutines.

To create options, use the WithXXX functions. You can supply options to `New`, `NewStream`. 
If no Options are supplied, default options are used.

Example of how to supply options:

 ```Go
     enc, err := reedsolomon.New(10, 3, WithMaxGoroutines(25))
 ```

# Leopard Compatible GF16

When you encode more than 256 shards the library will switch to a [Leopard-RS](https://github.com/catid/leopard) implementation.

This allows encoding up to 65536 shards (data+parity) with the following limitations, similar to leopard:

* The original and recovery data must not exceed 65536 pieces.
* The shard size *must*  each be a multiple of 64 bytes.
* Each buffer should have the same number of bytes.
* Even the last shard must be rounded up to the block size.

|                 | Regular | Leopard |
|-----------------|---------|---------|
| Encode          | ✓       | ✓       |
| EncodeIdx       | ✓       | -       |
| Verify          | ✓       | ✓       |
| Reconstruct     | ✓       | ✓       |
| ReconstructData | ✓       | ✓       |
| ReconstructSome | ✓       | ✓ (+)   |
| Update          | ✓       | -       |
| Split           | ✓       | ✓       |
| Join            | ✓       | ✓       |

* (+) Same as calling `ReconstructData`.

The Split/Join functions will help to split an input to the proper sizes.

Speed can be expected to be `O(N*log(N))`, compared to the `O(N*N)`. 
Reconstruction matrix calculation is more time-consuming, 
so be sure to include that as part of any benchmark you run.  

For now SSSE3, AVX2 and AVX512 assembly are available on AMD64 platforms.

Leopard mode currently always runs as a single goroutine, since multiple 
goroutines doesn't provide any worthwhile speedup.

## Leopard GF8

It is possible to replace the default reed-solomon encoder with a leopard compatible one.
This will typically be faster when dealing with more than 20-30 shards.
Note that the limitations listed above also applies to this mode. 
See table below for speed with different number of shards.

To enable Leopard GF8 mode use `WithLeopardGF(true)`.

Benchmark Encoding and Reconstructing *1KB* shards with variable number of shards.
All implementation use inversion cache when available.
Speed is total shard size for each operation. Data shard throughput is speed/2.
AVX2 is used.

| Encoder      | Shards      | Encode         | Recover All  | Recover One    |
|--------------|-------------|----------------|--------------|----------------|
| Cauchy       | 4+4         | 23076.83 MB/s  | 5444.02 MB/s | 10834.67 MB/s  |
| Cauchy       | 8+8         | 15206.87 MB/s  | 4223.42 MB/s | 16181.62  MB/s |
| Cauchy       | 16+16       | 7427.47 MB/s   | 3305.84 MB/s | 22480.41  MB/s |
| Cauchy       | 32+32       | 3785.64 MB/s   | 2300.07 MB/s | 26181.31  MB/s |
| Cauchy       | 64+64       | 1911.93 MB/s   | 1368.51 MB/s | 27992.93 MB/s  |
| Cauchy       | 128+128     | 963.83 MB/s    | 1327.56 MB/s | 32866.86 MB/s  |
| Leopard GF8  | 4+4         | 17061.28 MB/s  | 3099.06 MB/s | 4096.78 MB/s   |
| Leopard GF8  | 8+8         | 10546.67 MB/s  | 2925.92 MB/s | 3964.00 MB/s   |
| Leopard GF8  | 16+16       | 10961.37  MB/s | 2328.40 MB/s | 3110.22 MB/s   |
| Leopard GF8  | 32+32       | 7111.47 MB/s   | 2374.61 MB/s | 3220.75 MB/s   |
| Leopard GF8  | 64+64       | 7468.57 MB/s   | 2055.41 MB/s | 3061.81 MB/s   |
| Leopard GF8  | 128+128     | 5479.99 MB/s   | 1953.21 MB/s | 2815.15 MB/s   |
| Leopard GF16 | 256+256     | 6158.66 MB/s   | 454.14 MB/s  | 506.70 MB/s    |
| Leopard GF16 | 512+512     | 4418.58 MB/s   | 685.75 MB/s  | 801.63 MB/s    |
| Leopard GF16 | 1024+1024   | 4778.05 MB/s   | 814.51 MB/s  | 1080.19 MB/s   |
| Leopard GF16 | 2048+2048   | 3417.05 MB/s   | 911.64 MB/s  | 1179.48 MB/s   |
| Leopard GF16 | 4096+4096   | 3209.41 MB/s   | 729.13 MB/s  | 1135.06 MB/s   |
| Leopard GF16 | 8192+8192   | 2034.11 MB/s   | 604.52 MB/s  | 842.13 MB/s    |
| Leopard GF16 | 16384+16384 | 1525.88 MB/s   | 486.74 MB/s  | 750.01 MB/s    |
| Leopard GF16 | 32768+32768 | 1138.67 MB/s   | 482.81 MB/s  | 712.73 MB/s    |

"Traditional" encoding is faster until somewhere between 16 and 32 shards.
Leopard provides fast encoding in all cases, but shows a significant overhead for reconstruction.

Calculating the reconstruction matrix takes a significant amount of computation. 
With bigger shards that will be smaller. Arguably, fewer shards typically also means bigger shards.
Due to the high shard count caching reconstruction matrices generally isn't feasible for Leopard. 

# Performance

Performance depends mainly on the number of parity shards. 
In rough terms, doubling the number of parity shards will double the encoding time.

Here are the throughput numbers with some different selections of data and parity shards. 
For reference each shard is 1MB random data, and 16 CPU cores are used for encoding.

| Data | Parity | Go MB/s | SSSE3 MB/s | AVX2 MB/s |
|------|--------|---------|------------|-----------|
| 5    | 2      | 20,772  | 66,355     | 108,755   |
| 8    | 8      | 6,815   | 38,338     | 70,516    |
| 10   | 4      | 9,245   | 48,237     | 93,875    |
| 50   | 20     | 2,063   | 12,130     | 22,828    |

The throughput numbers here is the size of the encoded data and parity shards.

If `runtime.GOMAXPROCS()` is set to a value higher than 1, 
the encoder will use multiple goroutines to perform the calculations in `Verify`, `Encode` and `Reconstruct`.


Benchmarking `Reconstruct()` followed by a `Verify()` (=`all`) versus just calling `ReconstructData()` (=`data`) gives the following result:
```
benchmark                            all MB/s     data MB/s    speedup
BenchmarkReconstruct10x2x10000-8     2011.67      10530.10     5.23x
BenchmarkReconstruct50x5x50000-8     4585.41      14301.60     3.12x
BenchmarkReconstruct10x2x1M-8        8081.15      28216.41     3.49x
BenchmarkReconstruct5x2x1M-8         5780.07      28015.37     4.85x
BenchmarkReconstruct10x4x1M-8        4352.56      14367.61     3.30x
BenchmarkReconstruct50x20x1M-8       1364.35      4189.79      3.07x
BenchmarkReconstruct10x4x16M-8       1484.35      5779.53      3.89x
```

The package will use [GFNI](https://en.wikipedia.org/wiki/AVX-512#GFNI) instructions combined with AVX512 when these are available.
This further improves speed by up to 3x over AVX2 code paths.

## ARM64 NEON

By exploiting NEON instructions the performance for ARM has been accelerated. 
Below are the performance numbers for a single core on an EC2 m6g.16xlarge (Graviton2) instance (Amazon Linux 2):

```
BenchmarkGalois128K-64        119562     10028 ns/op        13070.78 MB/s
BenchmarkGalois1M-64           14380     83424 ns/op        12569.22 MB/s
BenchmarkGaloisXor128K-64      96508     12432 ns/op        10543.29 MB/s
BenchmarkGaloisXor1M-64        10000    100322 ns/op        10452.13 MB/s
```

# Performance on ppc64le

The performance for ppc64le has been accelerated. 
This gives roughly a 10x performance improvement on this architecture as can be seen below:

```
benchmark                      old MB/s     new MB/s     speedup
BenchmarkGalois128K-160        948.87       8878.85      9.36x
BenchmarkGalois1M-160          968.85       9041.92      9.33x
BenchmarkGaloisXor128K-160     862.02       7905.00      9.17x
BenchmarkGaloisXor1M-160       784.60       6296.65      8.03x
```

# Legal

> None of section below is legal advice. Seek your own legal counsel.
> As stated by the [LICENSE](LICENSE) the authors will not be held reliable for any use of this library.
> Users are encouraged to independently verify they comply with all legal requirements. 

As can be seen in [recent news](https://www.datanami.com/2023/10/16/cloudera-hit-with-240-million-judgement-over-erasure-coding/)
there has been lawsuits related to possible patents of aspects of erasure coding functionality.

As a possible mitigation it is possible to use the tag `nopshufb` when compiling any code which includes this package.
This will remove all inclusion and use of `PSHUFB` and equivalent on other platforms.

This is done by adding `-tags=nopshufb` to `go build` and similar commands that produce binary output.

The removed code may not be infringing and even after `-tags=nopshufb` there may still be infringing code left. 

# Links
* [Backblaze Open Sources Reed-Solomon Erasure Coding Source Code](https://www.backblaze.com/blog/reed-solomon/).
* [JavaReedSolomon](https://github.com/Backblaze/JavaReedSolomon). Compatible java library by Backblaze.
* [ocaml-reed-solomon-erasure](https://gitlab.com/darrenldl/ocaml-reed-solomon-erasure). Compatible OCaml implementation.
* [reedsolomon-c](https://github.com/jannson/reedsolomon-c). C version, compatible with output from this package.
* [Reed-Solomon Erasure Coding in Haskell](https://github.com/NicolasT/reedsolomon). Haskell port of the package with similar performance.
* [reed-solomon-erasure](https://github.com/darrenldl/reed-solomon-erasure). Compatible Rust implementation.
* [go-erasure](https://github.com/somethingnew2-0/go-erasure). A similar library using cgo, slower in my tests.
* [Screaming Fast Galois Field Arithmetic](http://www.snia.org/sites/default/files2/SDC2013/presentations/NewThinking/EthanMiller_Screaming_Fast_Galois_Field%20Arithmetic_SIMD%20Instructions.pdf). Basis for SSE3 optimizations.
* [Leopard-RS](https://github.com/catid/leopard) C library used as basis for GF16 implementation.

# License

This code, as the original [JavaReedSolomon](https://github.com/Backblaze/JavaReedSolomon) is published under an MIT license. See LICENSE file for more information.
