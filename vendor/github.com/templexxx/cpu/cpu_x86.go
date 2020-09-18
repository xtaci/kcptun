// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build 386 amd64 amd64p32

package cpu

const CacheLineSize = 64

// cpuid is implemented in cpu_x86.s.
func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)

// xgetbv with ecx = 0 is implemented in cpu_x86.s.
func xgetbv() (eax, edx uint32)

const (
	// edx bits
	cpuid_SSE2 = 1 << 26

	// ecx bits
	cpuid_SSE3      = 1 << 0
	cpuid_PCLMULQDQ = 1 << 1
	cpuid_SSSE3     = 1 << 9
	cpuid_FMA       = 1 << 12
	cpuid_SSE41     = 1 << 19
	cpuid_SSE42     = 1 << 20
	cpuid_POPCNT    = 1 << 23
	cpuid_AES       = 1 << 25
	cpuid_OSXSAVE   = 1 << 27
	cpuid_AVX       = 1 << 28

	// ebx bits
	cpuid_BMI1     = 1 << 3
	cpuid_AVX2     = 1 << 5
	cpuid_BMI2     = 1 << 8
	cpuid_ERMS     = 1 << 9
	cpuid_ADX      = 1 << 19
	cpuid_AVX512F  = 1 << 16
	cpuid_AVX512DQ = 1 << 17
	cpuid_AVX512BW = 1 << 30
	cpuid_AVX512VL = 1 << 31
)

func doinit() {
	options = []option{
		{"adx", &X86.HasADX},
		{"aes", &X86.HasAES},
		{"avx", &X86.HasAVX},
		{"avx2", &X86.HasAVX2},
		{"bmi1", &X86.HasBMI1},
		{"bmi2", &X86.HasBMI2},
		{"erms", &X86.HasERMS},
		{"fma", &X86.HasFMA},
		{"pclmulqdq", &X86.HasPCLMULQDQ},
		{"popcnt", &X86.HasPOPCNT},
		{"sse3", &X86.HasSSE3},
		{"sse41", &X86.HasSSE41},
		{"sse42", &X86.HasSSE42},
		{"ssse3", &X86.HasSSSE3},
		{"avx512f", &X86.HasAVX512F},
		{"avx512dq", &X86.HasAVX512DQ},
		{"avx512bw", &X86.HasAVX512BW},
		{"avx512vl", &X86.HasAVX512VL},

		// sse2 set as last element so it can easily be removed again. See code below.
		{"sse2", &X86.HasSSE2},
	}

	// Remove sse2 from options on amd64(p32) because SSE2 is a mandatory feature for these GOARCHs.
	if GOARCH == "amd64" || GOARCH == "amd64p32" {
		options = options[:len(options)-1]
	}

	maxID, _, _, _ := cpuid(0, 0)

	if maxID < 1 {
		return
	}

	_, _, ecx1, edx1 := cpuid(1, 0)
	X86.HasSSE2 = isSet(edx1, cpuid_SSE2)

	X86.HasSSE3 = isSet(ecx1, cpuid_SSE3)
	X86.HasPCLMULQDQ = isSet(ecx1, cpuid_PCLMULQDQ)
	X86.HasSSSE3 = isSet(ecx1, cpuid_SSSE3)
	X86.HasFMA = isSet(ecx1, cpuid_FMA)
	X86.HasSSE41 = isSet(ecx1, cpuid_SSE41)
	X86.HasSSE42 = isSet(ecx1, cpuid_SSE42)
	X86.HasPOPCNT = isSet(ecx1, cpuid_POPCNT)
	X86.HasAES = isSet(ecx1, cpuid_AES)
	X86.HasOSXSAVE = isSet(ecx1, cpuid_OSXSAVE)

	osSupportsAVX := false
	osSupportsAVX512 := false
	// For XGETBV, OSXSAVE bit is required and sufficient.
	if X86.HasOSXSAVE {
		eax, _ := xgetbv()
		// Check if XMM and YMM registers have OS support.
		osSupportsAVX = isSet(eax, 1<<1) && isSet(eax, 1<<2)
		// Check is ZMM registers have OS support.
		osSupportsAVX512 = isSet(eax>>5, 7) && isSet(eax>>1, 3)
	}

	X86.HasAVX = isSet(ecx1, cpuid_AVX) && osSupportsAVX

	if maxID < 7 {
		return
	}

	_, ebx7, _, _ := cpuid(7, 0)
	X86.HasBMI1 = isSet(ebx7, cpuid_BMI1)
	X86.HasAVX2 = isSet(ebx7, cpuid_AVX2) && osSupportsAVX
	X86.HasAVX512F = isSet(ebx7, cpuid_AVX512F) && osSupportsAVX512
	X86.HasAVX512DQ = isSet(ebx7, cpuid_AVX512DQ) && osSupportsAVX512
	X86.HasAVX512BW = isSet(ebx7, cpuid_AVX512BW) && osSupportsAVX512
	X86.HasAVX512VL = isSet(ebx7, cpuid_AVX512VL) && osSupportsAVX512
	X86.HasBMI2 = isSet(ebx7, cpuid_BMI2)
	X86.HasERMS = isSet(ebx7, cpuid_ERMS)
	X86.HasADX = isSet(ebx7, cpuid_ADX)

	X86.Cache = getCacheSize()
}

func isSet(hwc uint32, value uint32) bool {
	return hwc&value != 0
}

// getCacheSize is from
// https://github.com/klauspost/cpuid/blob/5a626f7029c910cc8329dae5405ee4f65034bce5/cpuid.go#L723
func getCacheSize() Cache {
	c := Cache{
		L1I: -1,
		L1D: -1,
		L2:  -1,
		L3:  -1,
	}

	vendor := vendorID()
	switch vendor {
	case Intel:
		if maxFunctionID() < 4 {
			return c
		}
		for i := uint32(0); ; i++ {
			eax, ebx, ecx, _ := cpuid(4, i)
			cacheType := eax & 15
			if cacheType == 0 {
				break
			}
			cacheLevel := (eax >> 5) & 7
			coherency := int(ebx&0xfff) + 1
			partitions := int((ebx>>12)&0x3ff) + 1
			associativity := int((ebx>>22)&0x3ff) + 1
			sets := int(ecx) + 1
			size := associativity * partitions * coherency * sets
			switch cacheLevel {
			case 1:
				if cacheType == 1 {
					// 1 = Data Cache
					c.L1D = size
				} else if cacheType == 2 {
					// 2 = Instruction Cache
					c.L1I = size
				} else {
					if c.L1D < 0 {
						c.L1I = size
					}
					if c.L1I < 0 {
						c.L1I = size
					}
				}
			case 2:
				c.L2 = size
			case 3:
				c.L3 = size
			}
		}
	case AMD, Hygon:
		// Untested.
		if maxExtendedFunction() < 0x80000005 {
			return c
		}
		_, _, ecx, edx := cpuid(0x80000005, 0)
		c.L1D = int(((ecx >> 24) & 0xFF) * 1024)
		c.L1I = int(((edx >> 24) & 0xFF) * 1024)

		if maxExtendedFunction() < 0x80000006 {
			return c
		}
		_, _, ecx, _ = cpuid(0x80000006, 0)
		c.L2 = int(((ecx >> 16) & 0xFFFF) * 1024)
	}

	return c
}

func maxFunctionID() uint32 {
	a, _, _, _ := cpuid(0, 0)
	return a
}

func maxExtendedFunction() uint32 {
	eax, _, _, _ := cpuid(0x80000000, 0)
	return eax
}

const (
	Other = iota
	Intel
	AMD
	VIA
	Transmeta
	NSC
	KVM  // Kernel-based Virtual Machine
	MSVM // Microsoft Hyper-V or Windows Virtual PC
	VMware
	XenHVM
	Bhyve
	Hygon
)

// Except from http://en.wikipedia.org/wiki/CPUID#EAX.3D0:_Get_vendor_ID
var vendorMapping = map[string]int{
	"AMDisbetter!": AMD,
	"AuthenticAMD": AMD,
	"CentaurHauls": VIA,
	"GenuineIntel": Intel,
	"TransmetaCPU": Transmeta,
	"GenuineTMx86": Transmeta,
	"Geode by NSC": NSC,
	"VIA VIA VIA ": VIA,
	"KVMKVMKVMKVM": KVM,
	"Microsoft Hv": MSVM,
	"VMwareVMware": VMware,
	"XenVMMXenVMM": XenHVM,
	"bhyve bhyve ": Bhyve,
	"HygonGenuine": Hygon,
}

func vendorID() int {
	_, b, c, d := cpuid(0, 0)
	v := valAsString(b, d, c)
	vend, ok := vendorMapping[string(v)]
	if !ok {
		return Other
	}
	return vend
}

func valAsString(values ...uint32) []byte {
	r := make([]byte, 4*len(values))
	for i, v := range values {
		dst := r[i*4:]
		dst[0] = byte(v & 0xff)
		dst[1] = byte((v >> 8) & 0xff)
		dst[2] = byte((v >> 16) & 0xff)
		dst[3] = byte((v >> 24) & 0xff)
		switch {
		case dst[0] == 0:
			return r[:i*4]
		case dst[1] == 0:
			return r[:i*4+1]
		case dst[2] == 0:
			return r[:i*4+2]
		case dst[3] == 0:
			return r[:i*4+3]
		}
	}
	return r
}
