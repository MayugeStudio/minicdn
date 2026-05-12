package main

import (
	"net/url"
	"github.com/dchest/siphash"
)

type Backend struct {
	// Name reprensents the name of the backend which is used to [Maglev]
	Name string 
	Addr url.URL
}

type Maglev struct {
	n int
	m uint64
	lookup []int

	offsets []uint64
	initialOffsets []uint64

	skips []uint64

	// dead stores dead backend indeces
	dead map[int]struct{}

	// backends stores an array of backends
	backends []*Backend
}


func New(backends []*Backend, m uint64) *Maglev {
	offsets, skips := generateOffsetsAndSkips(backends, m)
	maglev := &Maglev{
		n: len(backends),
		m: m,
		offsets: offsets,
		initialOffsets: make([]uint64, len(backends)),
		skips: skips,
		dead: make(map[int]struct{}),
		backends: backends,
	}

	copy(maglev.initialOffsets, offsets)
	maglev.lookup = maglev.populate()

	return maglev
}

func (p *Maglev) Lookup(key string) *Backend {
	h := siphash.Hash(0xdeadbeef, 0, []byte(key))
	idx := h % uint64(len(p.backends))
	return p.backends[p.lookup[idx]]
}

func (p *Maglev) Rebuild() {
	p.lookup = p.populate()
}

func (p *Maglev) Kill(i int) {
	p.dead[i] = struct{}{}
	p.Rebuild()
}

func (p *Maglev) Revive(i int) {
	delete(p.dead, i)
	p.Rebuild()
}

func (p *Maglev) populate() []int {
	entry := make([]int, p.m)
	for j := range entry {
		entry[j] = -1
	}

	var n uint64 = 0
	for {
		// for each backends
		for i := 0; i < len(p.backends); i++ {
			if _, exists := p.dead[i]; exists {
				continue
			}

			c := p.nextCandidate(i)
			for entry[c] >= 0 {
				c = p.nextCandidate(i)
			}

			entry[c] = i
			n++
			if n == p.m {
				return entry
			}
		}
	}

	return entry
}

func (p *Maglev) nextCandidate(i int) uint64 {
	res := p.offsets[i]

	p.offsets[i] += p.skips[i]
	if p.offsets[i] >= p.m {
		p.offsets[i] -= p.m
	}
	return res
} 

func generateOffsetsAndSkips(backends []*Backend, m uint64) ([]uint64, []uint64) {
	offsets := make([]uint64, len(backends))
	skips := make([]uint64, len(backends))

	for i, backend := range backends {
		b := []byte(backend.Name)
		h := siphash.Hash(0xdeadbeef, 0, b)
		// There is a small trick in here:
		// use upper 32 bits for offsets and lower 32 bits for skips,
		// effectively getting two hash functions from one siphash call.
		offsets[i] = (h>>32)%m
		skips[i] = (h&0xffffffff) % (m-1) + 1
	}

	return offsets, skips
}

