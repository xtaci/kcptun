# XOR SIMD

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Build Status][5]][6] [![Go Report Card][7]][8] [![Sourcegraph][9]][10]

[1]: https://godoc.org/github.com/templexxx/xorsimd?status.svg
[2]: https://godoc.org/github.com/templexxx/xorsimd
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://github.com/templexxx/xorsimd/workflows/unit-test/badge.svg
[6]: https://github.com/templexxx/xorsimd
[7]: https://goreportcard.com/badge/github.com/templexxx/xorsimd
[8]: https://goreportcard.com/report/github.com/templexxx/xorsimd
[9]: https://sourcegraph.com/github.com/templexxx/xorsimd/-/badge.svg
[10]: https://sourcegraph.com/github.com/templexxx/xorsimd?badge

## Introduction:

>- XOR code engine in pure Go.
>
>- [High Performance](https://github.com/templexxx/xorsimd#performance): 
More than 270GB/s per physics core. 

## Performance

Performance depends mainly on:

>- CPU instruction extension.
>
>- Number of source row vectors.

**Platform:** 

*AWS c5d.xlarge (Intel(R) Xeon(R) Platinum 8124M CPU @ 3.00GHz)*

**All test run on a single Core.**

`I/O = (src_num + 1) * vector_size / cost`

| Src Num  | Vector size | AVX512 I/O (MB/S) |  AVX2 I/O (MB/S) |SSE2 I/O (MB/S) |
|-------|-------------|-------------|---------------|---------------|
|5|4KB|     270403.73    |     142825.25    |    74443.91    |
|5|1MB|    26948.34     |   26887.37 	      |     26950.65     | 
|5|8MB|     17881.32     |    17212.56      |  16402.97      | 
|10|4KB|     190445.30    |   102953.59      |   53244.04       |  
|10|1MB|   26424.44     |     26618.65   |    26094.39    |   
|10|8MB|   15471.31      |     14866.72      |    13565.80      |  
