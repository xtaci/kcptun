// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build 386 amd64 amd64p32

package cpu

import (
	"fmt"
	"strings"
)

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
	cpuid_CMPXCHG16B = 1 << 13

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

	// edx bits
	cpuid_Invariant_TSC = 1 << 8
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
		{"invariant_tsc", &X86.HasInvariantTSC},

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
	X86.HasCMPXCHG16B = isSet(ecx1, cpuid_CMPXCHG16B)
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

	X86.HasInvariantTSC = hasInvariantTSC()

	X86.Family, X86.Model, X86.SteppingID = getVersionInfo()

	X86.Signature = makeSignature(X86.Family, X86.Model)

	X86.Name = getName()

	X86.TSCFrequency = getNativeTSCFrequency(X86.Name, X86.Signature, X86.SteppingID)
}

func isSet(hwc uint32, value uint32) bool {
	return hwc&value != 0
}

func hasInvariantTSC() bool {
	if maxExtendedFunction() < 0x80000007 {
		return false
	}
	_, _, _, edx := cpuid(0x80000007, 0)
	return isSet(edx, cpuid_Invariant_TSC)
}

func getName() string {
	if maxExtendedFunction() >= 0x80000004 {
		v := make([]uint32, 0, 48)
		for i := uint32(0); i < 3; i++ {
			a, b, c, d := cpuid(0x80000002+i, 0)
			v = append(v, a, b, c, d)
		}
		return strings.Trim(string(valAsString(v...)), " ")
	}
	return "unknown"
}

// getNativeTSCFrequency gets TSC frequency from CPUID,
// only supports Intel (Skylake or later microarchitecture) & key information is from Intel manual & kernel codes
// (especially this commit: https://github.com/torvalds/linux/commit/604dc9170f2435d27da5039a3efd757dceadc684).
func getNativeTSCFrequency(name, sign string, steppingID uint32) uint64 {

	if vendorID() != Intel {
		return 0
	}

	if maxFunctionID() < 0x15 {
		return 0
	}

	// ApolloLake, GeminiLake, CannonLake (and presumably all new chipsets
	// from this point) report the crystal frequency directly via CPUID.0x15.
	// That's definitive data that we can rely upon.
	eax, ebx, ecx, _ := cpuid(0x15, 0)

	// If ebx is 0, the TSC/”core crystal clock” ratio is not enumerated.
	// We won't provide TSC frequency detection in this situation.
	if eax == 0 || ebx == 0 {
		return 0
	}

	// Skylake, Kabylake and all variants of those two chipsets report a
	// crystal frequency of zero.
	if ecx == 0 { // Crystal clock frequency is not enumerated.
		ecx = getCrystalClockFrequency(sign, steppingID)
	}

	// TSC frequency = “core crystal clock frequency” * EBX/EAX.
	return uint64(ecx) * (uint64(ebx) / uint64(eax))
}

// Copied from: CPUID Signature values of DisplayFamily and DisplayModel,
// in Intel® 64 and IA-32 Architectures Software Developer’s Manual
// Volume 4: Model-Specific Registers
// & https://github.com/torvalds/linux/blob/master/arch/x86/include/asm/intel-family.h
const (
	IntelFam6SkylakeL     = "06_4EH"
	IntelFam6Skylake      = "06_5EH"
	IntelFam6XeonScalable = "06_55H"
	IntelFam6KabylakeL    = "06_8EH"
	IntelFam6Kabylake     = "06_9EH"
)

// getCrystalClockFrequency gets crystal clock frequency
// for Intel processors in which CPUID.15H.EBX[31:0] ÷ CPUID.0x15.EAX[31:0] is enumerated
// but CPUID.15H.ECX is not enumerated using this function to get nominal core crystal clock frequency.
//
// Actually these crystal clock frequencies provided by Intel hardcoded tables are not so accurate in some cases,
// e.g. SkyLake server CPU may have issue (All SKX subject the crystal to an EMI reduction circuit that
//reduces its actual frequency by (approximately) -0.25%):
// see https://lore.kernel.org/lkml/ff6dcea166e8ff8f2f6a03c17beab2cb436aa779.1513920414.git.len.brown@intel.com/
// for more details.
// With this report, I set a coefficient (0.9975) for IntelFam6SkyLakeX.
//
// Unlike the kernel way (mentioned in https://github.com/torvalds/linux/commit/604dc9170f2435d27da5039a3efd757dceadc684),
// I prefer the Intel hardcoded tables, (in <Intel® 64 and IA-32 Architectures Software Developer’s Manual, Volume 3>
// 18.7.3 Determining the Processor Base Frequency, Table 18-85. Nominal Core Crystal Clock Frequency)
// because after some testing (comparing with wall clock, see https://github.com/templexxx/tsc/tsc_test.go for more details),
// I found hardcoded tables are more accurate.
func getCrystalClockFrequency(sign string, steppingID uint32) uint32 {

	if maxFunctionID() < 0x16 {
		return 0
	}

	switch sign {
	case IntelFam6SkylakeL:
		return 24 * 1000 * 1000
	case IntelFam6Skylake:
		return 24 * 1000 * 1000
	case IntelFam6XeonScalable:
		// SKL-SP.
		// see: https://community.intel.com/t5/Software-Tuning-Performance/How-to-detect-microarchitecture-on-Xeon-Scalable/m-p/1205162#M7633.
		if steppingID == 0x2 || steppingID == 0x3 || steppingID == 0x4 {
			return 25 * 1000 * 1000 * 0.9975
		}
		return 25 * 1000 * 1000 // TODO check other Xeon Scalable has no slow down issue.
	case IntelFam6KabylakeL:
		return 24 * 1000 * 1000
	case IntelFam6Kabylake:
		return 24 * 1000 * 1000
	}

	return 0
}

func getVersionInfo() (uint32, uint32, uint32) {
	if maxFunctionID() < 0x1 {
		return 0, 0, 0
	}
	eax, _, _, _ := cpuid(1, 0)
	family := (eax >> 8) & 0xf
	displayFamily := family
	if family == 0xf {
		displayFamily = ((eax >> 20) & 0xff) + family
	}
	model := (eax >> 4) & 0xf
	displayModel := model
	if family == 0x6 || family == 0xf {
		displayModel = ((eax >> 12) & 0xf0) + model
	}
	return displayFamily, displayModel, eax & 0x7
}

// signature format: XX_XXH
func makeSignature(family, model uint32) string {
	signature := strings.ToUpper(fmt.Sprintf("0%x_0%xH", family, model))
	ss := strings.Split(signature, "_")
	for i, s := range ss {
		// Maybe insert too more `0`, drop it.
		if len(s) > 2 {
			s = s[1:]
			ss[i] = s
		}
	}
	return strings.Join(ss, "_")
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
