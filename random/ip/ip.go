package ip

import (
	"fmt"
	"net/netip"

	"github.com/mkeeler/consul-load-test/random/internal/options"
)

var (
	testingIPv4BaseAddr   = netip.MustParseAddr("198.18.0.0")
	testingIPv4PrefixBits = 15

	testingIPv4Prefix netip.Prefix

	bitMasks = map[int]uint8{
		0: 0b11111111,
		1: 0b01111111,
		2: 0b00111111,
		3: 0b00011111,
		4: 0b00001111,
		5: 0b00000111,
		6: 0b00000011,
		7: 0b00000001,
	}
)

func init() {
	var err error
	testingIPv4Prefix, err = testingIPv4BaseAddr.Prefix(testingIPv4PrefixBits)
	if err != nil {
		panic(fmt.Errorf("failed to create testing IPv4 prefix: %w", err))
	}
}

// TestingIPGenerator will generate IP addresses in the range 198.18.0.0 -198.19.255.255 which
// is reserved for testing purposes by the IANA
func NewTestingIPv4Generator(opts ...options.GeneratorOption) *IPPrefixGenerator {
	return NewIPPrefixGenerator(testingIPv4Prefix, opts...)
}

type IPPrefixGenerator struct {
	*options.GeneratorOptions
	prefix netip.Prefix
	mask   []byte
	ipv6   bool
}

func NewIPPrefixGenerator(prefix netip.Prefix, opts ...options.GeneratorOption) *IPPrefixGenerator {
	ipv6 := prefix.Addr().Is6()

	byteLen := 4
	if ipv6 {
		byteLen = 16
	}

	g := &IPPrefixGenerator{
		GeneratorOptions: options.ApplyGeneratorOptions(opts),
		prefix:           prefix.Masked(),
		ipv6:             ipv6,
		mask:             make([]byte, byteLen),
	}

	// calculate the bitmask to apply to the randome bytes we generate
	bitsLeft := prefix.Bits()
	for i := 0; i < len(g.mask); i++ {
		if bitsLeft >= 8 {
			g.mask[i] = 0
			bitsLeft -= 8
		} else {
			g.mask[i] = bitMasks[bitsLeft]
			bitsLeft = 0
		}
	}

	return g
}

func (g *IPPrefixGenerator) GenerateIP() netip.Addr {
	if g.ipv6 {
		return g.generateIPv6()
	} else {
		return g.generateIPv4()
	}
}

func (g *IPPrefixGenerator) generateIPv4() netip.Addr {
	randomValue := g.RNG.Uint32()
	randomBytes := [4]uint8{
		uint8(randomValue & 0xFF),
		uint8(randomValue >> 8 & 0xFF),
		uint8(randomValue >> 16 & 0xFF),
		uint8(randomValue >> 24 & 0xFF),
	}
	addrBytes := g.prefix.Addr().As4()

	addrBytes[0] = addrBytes[0] | (randomBytes[0] & g.mask[0])
	addrBytes[1] = addrBytes[1] | (randomBytes[1] & g.mask[1])
	addrBytes[2] = addrBytes[2] | (randomBytes[2] & g.mask[2])
	addrBytes[3] = addrBytes[3] | (randomBytes[3] & g.mask[3])

	return netip.AddrFrom4(addrBytes)
}

func (g *IPPrefixGenerator) generateIPv6() netip.Addr {
	randomValues := [2]uint64{
		g.RNG.Uint64(),
		g.RNG.Uint64(),
	}
	randomBytes := [16]uint8{
		uint8(randomValues[0] & 0xFF),
		uint8(randomValues[0] >> 8 & 0xFF),
		uint8(randomValues[0] >> 16 & 0xFF),
		uint8(randomValues[0] >> 24 & 0xFF),
		uint8(randomValues[0] >> 32 & 0xFF),
		uint8(randomValues[0] >> 40 & 0xFF),
		uint8(randomValues[0] >> 48 & 0xFF),
		uint8(randomValues[0] >> 56 & 0xFF),
		uint8(randomValues[1] & 0xFF),
		uint8(randomValues[1] >> 8 & 0xFF),
		uint8(randomValues[1] >> 16 & 0xFF),
		uint8(randomValues[1] >> 24 & 0xFF),
		uint8(randomValues[1] >> 32 & 0xFF),
		uint8(randomValues[1] >> 40 & 0xFF),
		uint8(randomValues[1] >> 48 & 0xFF),
		uint8(randomValues[1] >> 56 & 0xFF),
	}

	addrBytes := g.prefix.Addr().As16()

	for i := 0; i < 16; i++ {
		addrBytes[i] = addrBytes[i] | (randomBytes[i] & g.mask[i])
	}

	return netip.AddrFrom16(addrBytes)
}
