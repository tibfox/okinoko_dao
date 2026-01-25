package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"sort"

	"okinoko_dao/sdk"
)

type binWriter struct {
	buf bytes.Buffer
}

// newWriter spins up a fresh writer so we dont leak old bytes between encodes.
func newWriter() *binWriter { return &binWriter{} }

// bytes returns the accumulated buffer, tiny helper but keeps code tidy.
func (w *binWriter) bytes() []byte { return w.buf.Bytes() }

// writeBool squashes bools into a single byte flag for deterministic payloads.
func (w *binWriter) writeBool(v bool) {
	if v {
		w.buf.WriteByte(1)
	} else {
		w.buf.WriteByte(0)
	}
}

// writeUint64 writes big endian numbers so tooling can read them without guessing.
func (w *binWriter) writeUint64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	w.buf.Write(b[:])
}

// writeInt64 reuses the uint routine since casting keeps the sign bits intact.
func (w *binWriter) writeInt64(v int64) {
	w.writeUint64(uint64(v))
}

// writeFloat64 converts doubles to IEEE bits so we dont lose precision on wasm.
func (w *binWriter) writeFloat64(v float64) {
	w.writeUint64(math.Float64bits(v))
}

// writeVarUint uses varints to keep counts and lens compact.
func (w *binWriter) writeVarUint(v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	w.buf.Write(tmp[:n])
}

// writeVarInt mirrors writeVarUint but keeps sign info for deltas.
func (w *binWriter) writeVarInt(v int64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutVarint(tmp[:], v)
	w.buf.Write(tmp[:n])
}

// writeAmount keeps amount scaling consistent via a single call site.
func (w *binWriter) writeAmount(v Amount) {
	w.writeInt64(int64(v))
}

// writeString prefixes its length then dumps UTF-8 directly.
func (w *binWriter) writeString(s string) {
	w.writeVarUint(uint64(len(s)))
	w.buf.WriteString(s)
}

// writeOptionalString writes a presence bit so decoders know if data follows.
func (w *binWriter) writeOptionalString(ptr *string) {
	if ptr == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeString(*ptr)
}

// writeOptionalUint64 does the same dance for numeric ids.
func (w *binWriter) writeOptionalUint64(ptr *uint64) {
	if ptr == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeUint64(*ptr)
}

// writeStringMap iterates keys in sorted order so binary blobs are stable.
func (w *binWriter) writeStringMap(m map[string]string) {
	if m == nil {
		w.writeVarUint(0)
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w.writeVarUint(uint64(len(keys)))
	for _, k := range keys {
		w.writeString(k)
		w.writeString(m[k])
	}
}

// writeAddress canonicalizes the address before writing, so later parsing is easyer.
func (w *binWriter) writeAddress(a sdk.Address) {
	w.writeString(AddressToString(a))
}

// writeAsset just dumps the ticker string, nothing fancy but consistent.
func (w *binWriter) writeAsset(a sdk.Asset) {
	w.writeString(AssetToString(a))
}

// encodeProjectConfig squeezes every config bit into the binary form, kinda verbose but fast.
func encodeProjectConfig(w *binWriter, cfg *ProjectConfig) {
	w.buf.WriteByte(byte(cfg.VotingSystem))
	w.writeFloat64(cfg.ThresholdPercent)
	w.writeFloat64(cfg.QuorumPercent)
	w.writeUint64(cfg.ProposalDurationHours)
	w.writeUint64(cfg.ExecutionDelayHours)
	w.writeUint64(cfg.LeaveCooldownHours)
	w.writeFloat64(cfg.ProposalCost)
	w.writeFloat64(cfg.StakeMinAmt)
	w.writeOptionalString(cfg.MembershipNFTContract)
	w.writeOptionalString(cfg.MembershipNFTContractFunction)
	w.writeOptionalUint64(cfg.MembershipNFT)
	w.writeString(cfg.MembershipNftPayloadFormat)
	w.writeBool(cfg.ProposalsMembersOnly)
	w.writeBool(cfg.WhitelistOnly)
}

// encodeMember serializes member lifecycle data for caching and rehydrating later.
func encodeMember(w *binWriter, m *Member) {
	w.writeAddress(m.Address)
	w.writeAmount(m.Stake)
	w.writeInt64(m.JoinedAt)
	w.writeInt64(m.LastActionAt)
	w.writeInt64(m.ExitRequested)
	w.writeInt64(m.Reputation)
	w.writeUint64(m.StakeIncrement)
}

// EncodeMember packs a Member into bytes so storage stays lean and no json noise leaks.
// Example payload: EncodeMember(&Member{Address: AddressFromString("hive:alice"), Stake: FloatToAmount(3.5)})
func EncodeMember(m *Member) []byte {
	w := newWriter()
	encodeMember(w, m)
	return w.bytes()
}

// encodeProposalOption captures text, URL, cumulative weight and unique voter count.
func encodeProposalOption(w *binWriter, opt *ProposalOption) {
	w.writeString(opt.Text)
	w.writeString(opt.URL)
	w.writeAmount(opt.WeightTotal)
	w.writeUint64(opt.VoterCount)
}

// encodeProposalOutcome first toggles presence, then writes meta map and payout slice.
func encodeProposalOutcome(w *binWriter, out *ProposalOutcome) {
	if out == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeStringMap(out.Meta)
	// Write payout slice (supports multiple entries per address with different assets)
	w.writeVarUint(uint64(len(out.Payout)))
	for _, entry := range out.Payout {
		w.writeAddress(entry.Address)
		w.writeAmount(entry.Amount)
		w.writeAsset(entry.Asset)
	}
	// Encode ICC calls
	w.writeVarUint(uint64(len(out.ICC)))
	for _, icc := range out.ICC {
		w.writeString(icc.ContractAddress)
		w.writeString(icc.Function)
		w.writeString(icc.Payload)
		// Encode assets map
		if icc.Assets == nil {
			w.writeVarUint(0)
		} else {
			// Sort assets for deterministic encoding
			assetStrs := make([]string, 0, len(icc.Assets))
			for asset := range icc.Assets {
				assetStrs = append(assetStrs, AssetToString(asset))
			}
			sort.Strings(assetStrs)
			w.writeVarUint(uint64(len(assetStrs)))
			for _, assetStr := range assetStrs {
				w.writeString(assetStr)
				w.writeAmount(icc.Assets[AssetFromString(assetStr)])
			}
		}
	}
}

// EncodeProject serializes the entire Project into deterministic bytes for storage and proofs.
// Example payload: EncodeProject(&Project{ID: 7, Name: "Tiny DAO", Funds: FloatToAmount(2.5)})
func EncodeProject(prj *Project) []byte {
	w := newWriter()
	w.writeUint64(prj.ID)
	w.writeAddress(prj.Owner)
	w.writeString(prj.Name)
	w.writeString(prj.Description)
	encodeProjectConfig(w, &prj.Config)
	w.writeAsset(prj.FundsAsset)
	w.writeBool(prj.Paused)
	w.writeString(prj.Tx)
	w.writeString(prj.Metadata)
	w.writeAmount(prj.StakeTotal)
	w.writeUint64(prj.MemberCount)
	w.writeString(prj.URL)
	return w.bytes()
}

// EncodeProposal turns a Proposal into bytes so we can persist votes without json overhead.
// Example payload: EncodeProposal(&Proposal{ID: 3, ProjectID: 1, Name: "add funds"})
func EncodeProposal(prpsl *Proposal) []byte {
	w := newWriter()
	w.writeUint64(prpsl.ID)
	w.writeUint64(prpsl.ProjectID)
	w.writeAddress(prpsl.Creator)
	w.writeString(prpsl.Name)
	w.writeString(prpsl.Description)
	w.writeUint64(uint64(prpsl.OptionCount))
	w.writeUint64(prpsl.DurationHours)
	w.writeInt64(prpsl.CreatedAt)
	w.buf.WriteByte(byte(prpsl.State))
	encodeProposalOutcome(w, prpsl.Outcome)
	w.writeString(prpsl.Tx)
	w.writeAmount(prpsl.StakeSnapshot)
	w.writeVarUint(uint64(prpsl.MemberCountSnapshot))
	w.writeString(prpsl.Metadata)
	w.writeBool(prpsl.IsPoll)
	w.writeVarInt(int64(prpsl.ResultOptionID))
	w.writeInt64(prpsl.ExecutableAt)
	w.writeString(prpsl.URL)
	return w.bytes()
}

// EncodeCreateProjectArgs packs the human payload for project_create into deterministic bytes.
// Example payload: EncodeCreateProjectArgs(&CreateProjectArgs{Name: "My Coop", ProjectConfig: ProjectConfig{ThresholdPercent:60}})
func EncodeCreateProjectArgs(args *CreateProjectArgs) []byte {
	w := newWriter()
	w.writeString(args.Name)
	w.writeString(args.Description)
	encodeProjectConfig(w, &args.ProjectConfig)
	w.writeString(args.Metadata)
	w.writeString(args.URL)
	return w.bytes()
}

// EncodeCreateProposalArgs is used in tests/tools to mimic proposal_create payloads.
// Example payload: EncodeCreateProposalArgs(&CreateProposalArgs{ProjectID:2, OptionsList: []ProposalOptionInput{{Text:"yes"},{Text:"no"}}})
func EncodeCreateProposalArgs(args *CreateProposalArgs) []byte {
	w := newWriter()
	w.writeUint64(args.ProjectID)
	w.writeString(args.Name)
	w.writeString(args.Description)
	w.writeVarUint(uint64(len(args.OptionsList)))
	for _, opt := range args.OptionsList {
		w.writeString(opt.Text)
		w.writeString(opt.URL)
	}
	encodeProposalOutcome(w, args.ProposalOutcome)
	w.writeUint64(args.ProposalDuration)
	w.writeString(args.Metadata)
	w.writeString(args.URL)
	return w.bytes()
}

// EncodeVoteProposalArgs mirrors the on-chain vote payload format for fuzzing etc.
// Example payload: EncodeVoteProposalArgs(&VoteProposalArgs{ProposalID:5, Choices: []uint{1,2}})
func EncodeVoteProposalArgs(args *VoteProposalArgs) []byte {
	w := newWriter()
	w.writeUint64(args.ProposalID)
	w.writeVarUint(uint64(len(args.Choices)))
	for _, choice := range args.Choices {
		w.writeVarUint(uint64(choice))
	}
	return w.bytes()
}

// EncodeAddFundsArgs lets tooling craft project_funds payloads fast.
// Example payload: EncodeAddFundsArgs(&AddFundsArgs{ProjectID:9, ToStake:true})
func EncodeAddFundsArgs(args *AddFundsArgs) []byte {
	w := newWriter()
	w.writeUint64(args.ProjectID)
	w.writeBool(args.ToStake)
	return w.bytes()
}

// ------------------------------------------------------------------
// Decoder helpers
// ------------------------------------------------------------------

type binReader struct {
	data []byte
	pos  int
}

// newReader wraps raw bytes so we can peek sequentially w/out copying.
func newReader(data []byte) *binReader {
	return &binReader{data: data}
}

// readByte grabs the next byte and bumps the cursor, aborts on EOF.
func (r *binReader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("unexpected EOF")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

// readBool restores bools stored via writeBool above.
func (r *binReader) readBool() (bool, error) {
	b, err := r.readByte()
	if err != nil {
		return false, err
	}
	return b == 1, nil
}

// readUint64 decodes big endian integers for ids and totals.
func (r *binReader) readUint64() (uint64, error) {
	if r.pos+8 > len(r.data) {
		return 0, errors.New("unexpected EOF")
	}
	val := binary.BigEndian.Uint64(r.data[r.pos : r.pos+8])
	r.pos += 8
	return val, nil
}

// readInt64 simply casts the unsigned read, matching the writer logic.
func (r *binReader) readInt64() (int64, error) {
	v, err := r.readUint64()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

// readFloat64 flips IEEE bits back into a go float.
func (r *binReader) readFloat64() (float64, error) {
	v, err := r.readUint64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

// readVarUint undoes the compact varint encoding for lengths/counts.
func (r *binReader) readVarUint() (uint64, error) {
	val, n := binary.Uvarint(r.data[r.pos:])
	if n <= 0 {
		return 0, errors.New("invalid varuint")
	}
	r.pos += n
	return val, nil
}

// readVarInt mirrors varuint for signed values.
func (r *binReader) readVarInt() (int64, error) {
	val, n := binary.Varint(r.data[r.pos:])
	if n <= 0 {
		return 0, errors.New("invalid varint")
	}
	r.pos += n
	return val, nil
}

// readAmount rebuilds a Amount using the int64 path so scaling stays synced.
func (r *binReader) readAmount() (Amount, error) {
	val, err := r.readInt64()
	if err != nil {
		return 0, err
	}
	return Amount(val), nil
}

func (r *binReader) readAsset() (sdk.Asset, error) {
	s, err := r.readString()
	if err != nil {
		return sdk.Asset(""), err
	}
	return AssetFromString(s), nil
}

// readString reads the varint length then slices out the utf8 chunk.
func (r *binReader) readString() (string, error) {
	l, err := r.readVarUint()
	if err != nil {
		return "", err
	}
	if r.pos+int(l) > len(r.data) {
		return "", errors.New("unexpected EOF")
	}
	s := string(r.data[r.pos : r.pos+int(l)])
	r.pos += int(l)
	return s, nil
}

// readOptionalString checks the presence byte, then returns pointer so callers know nil.
func (r *binReader) readOptionalString() (*string, error) {
	ok, err := r.readBool()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	str, err := r.readString()
	if err != nil {
		return nil, err
	}
	return &str, nil
}

// readOptionalUint64 is used for optional nft ids and similar numbers.
func (r *binReader) readOptionalUint64() (*uint64, error) {
	ok, err := r.readBool()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	val, err := r.readUint64()
	if err != nil {
		return nil, err
	}
	return &val, nil
}

// readStringMap loops len times and rebuilds the deterministic meta map.
func (r *binReader) readStringMap() (map[string]string, error) {
	count, err := r.readVarUint()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return map[string]string{}, nil
	}
	result := make(map[string]string, count)
	for i := uint64(0); i < count; i++ {
		key, err := r.readString()
		if err != nil {
			return nil, err
		}
		val, err := r.readString()
		if err != nil {
			return nil, err
		}
		result[key] = val
	}
	return result, nil
}

// decodeProjectConfig is the inverse of encodeProjectConfig and keeps same field order.
func decodeProjectConfig(r *binReader) (ProjectConfig, error) {
	var cfg ProjectConfig
	b, err := r.readByte()
	if err != nil {
		return cfg, err
	}
	cfg.VotingSystem = VotingSystem(b)
	if cfg.ThresholdPercent, err = r.readFloat64(); err != nil {
		return cfg, err
	}
	if cfg.QuorumPercent, err = r.readFloat64(); err != nil {
		return cfg, err
	}
	if cfg.ProposalDurationHours, err = r.readUint64(); err != nil {
		return cfg, err
	}
	if cfg.ExecutionDelayHours, err = r.readUint64(); err != nil {
		return cfg, err
	}
	if cfg.LeaveCooldownHours, err = r.readUint64(); err != nil {
		return cfg, err
	}
	if cfg.ProposalCost, err = r.readFloat64(); err != nil {
		return cfg, err
	}
	if cfg.StakeMinAmt, err = r.readFloat64(); err != nil {
		return cfg, err
	}
	if cfg.MembershipNFTContract, err = r.readOptionalString(); err != nil {
		return cfg, err
	}
	if cfg.MembershipNFTContractFunction, err = r.readOptionalString(); err != nil {
		return cfg, err
	}
	if cfg.MembershipNFT, err = r.readOptionalUint64(); err != nil {
		return cfg, err
	}
	if cfg.MembershipNftPayloadFormat, err = r.readString(); err != nil {
		return cfg, err
	}
	if cfg.ProposalsMembersOnly, err = r.readBool(); err != nil {
		return cfg, err
	}
	if r.pos < len(r.data) {
		if cfg.WhitelistOnly, err = r.readBool(); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// decodeMember reads back the fields emitted by encodeMember in exact order.
func decodeMember(r *binReader) (Member, error) {
	var m Member
	addr, err := r.readString()
	if err != nil {
		return m, err
	}
	m.Address = AddressFromString(addr)
	if m.Stake, err = r.readAmount(); err != nil {
		return m, err
	}
	if m.JoinedAt, err = r.readInt64(); err != nil {
		return m, err
	}
	if m.LastActionAt, err = r.readInt64(); err != nil {
		return m, err
	}
	if m.ExitRequested, err = r.readInt64(); err != nil {
		return m, err
	}
	if m.Reputation, err = r.readInt64(); err != nil {
		return m, err
	}
	// Read StakeIncrement (backwards compatible - defaults to 0 if missing)
	if r.pos < len(r.data) {
		if m.StakeIncrement, err = r.readUint64(); err != nil {
			return m, err
		}
	}
	return m, nil
}

// DecodeMember is handy for tests that need to inspect stored members quickly.
// Example payload: DecodeMember(EncodeMember(&Member{Address:AddressFromString("hive:tester")}))
func DecodeMember(data []byte) (*Member, error) {
	r := newReader(data)
	m, err := decodeMember(r)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// decodeProposalOption reconstructs the text option, URL, plus running vote totals.
func decodeProposalOption(r *binReader) (ProposalOption, error) {
	var opt ProposalOption
	var err error
	if opt.Text, err = r.readString(); err != nil {
		return opt, err
	}
	if opt.URL, err = r.readString(); err != nil {
		return opt, err
	}
	if opt.WeightTotal, err = r.readAmount(); err != nil {
		return opt, err
	}
	if opt.VoterCount, err = r.readUint64(); err != nil {
		return opt, err
	}
	return opt, nil
}

// EncodeProposalOption serializes a single option so we can persist it separately in state.
// Example payload: EncodeProposalOption(&ProposalOption{Text: "ship it", WeightTotal: FloatToAmount(12)})
func EncodeProposalOption(opt *ProposalOption) []byte {
	w := newWriter()
	encodeProposalOption(w, opt)
	return w.bytes()
}

// DecodeProposalOption lets tests read back stored vote options.
// Example payload: DecodeProposalOption(EncodeProposalOption(&ProposalOption{Text:"x"}))
func DecodeProposalOption(data []byte) (*ProposalOption, error) {
	r := newReader(data)
	opt, err := decodeProposalOption(r)
	if err != nil {
		return nil, err
	}
	return &opt, nil
}

// decodeProposalOutcome rebuilds optional meta and payout slice for proposals.
func decodeProposalOutcome(r *binReader) (*ProposalOutcome, error) {
	exists, err := r.readBool()
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	meta, err := r.readStringMap()
	if err != nil {
		return nil, err
	}
	count, err := r.readVarUint()
	if err != nil {
		return nil, err
	}
	// Read payout slice (supports multiple entries per address with different assets)
	payouts := make([]PayoutEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		addr, err := r.readString()
		if err != nil {
			return nil, err
		}
		value, err := r.readAmount()
		if err != nil {
			return nil, err
		}
		asset, err := r.readAsset()
		if err != nil {
			return nil, err
		}
		payouts = append(payouts, PayoutEntry{Address: AddressFromString(addr), Amount: value, Asset: asset})
	}

	// Decode ICC calls (backwards compatible - if missing, defaults to nil)
	var iccCalls []InterContractCall
	if r.pos < len(r.data) {
		iccCount, err := r.readVarUint()
		if err != nil {
			return nil, err
		}
		if iccCount > 0 {
			iccCalls = make([]InterContractCall, iccCount)
			for i := uint64(0); i < iccCount; i++ {
				contractAddr, err := r.readString()
				if err != nil {
					return nil, err
				}
				function, err := r.readString()
				if err != nil {
					return nil, err
				}
				payload, err := r.readString()
				if err != nil {
					return nil, err
				}
				// Decode assets map
				assetCount, err := r.readVarUint()
				if err != nil {
					return nil, err
				}
				var assets map[sdk.Asset]Amount
				if assetCount > 0 {
					assets = make(map[sdk.Asset]Amount, assetCount)
					for j := uint64(0); j < assetCount; j++ {
						assetStr, err := r.readString()
						if err != nil {
							return nil, err
						}
						amount, err := r.readAmount()
						if err != nil {
							return nil, err
						}
						assets[AssetFromString(assetStr)] = amount
					}
				}
				iccCalls[i] = InterContractCall{
					ContractAddress: contractAddr,
					Function:        function,
					Payload:         payload,
					Assets:          assets,
				}
			}
		}
	}

	return &ProposalOutcome{
		Meta:   meta,
		Payout: payouts,
		ICC:    iccCalls,
	}, nil
}

// DecodeProject lets off-chain tools verify stored projects without reimplementing codec.
// Example payload: DecodeProject(EncodeProject(&Project{ID:42, Name:"dao"}))
func DecodeProject(data []byte) (*Project, error) {
	r := newReader(data)
	prj := &Project{}
	var err error
	if prj.ID, err = r.readUint64(); err != nil {
		return nil, err
	}
	if owner, err := r.readString(); err == nil {
		prj.Owner = AddressFromString(owner)
	} else {
		return nil, err
	}
	if prj.Name, err = r.readString(); err != nil {
		return nil, err
	}
	if prj.Description, err = r.readString(); err != nil {
		return nil, err
	}
	if prj.Config, err = decodeProjectConfig(r); err != nil {
		return nil, err
	}
	if asset, err := r.readString(); err == nil {
		prj.FundsAsset = AssetFromString(asset)
	} else {
		return nil, err
	}
	if prj.Paused, err = r.readBool(); err != nil {
		return nil, err
	}
	if prj.Tx, err = r.readString(); err != nil {
		return nil, err
	}
	if prj.Metadata, err = r.readString(); err != nil {
		return nil, err
	}
	if prj.StakeTotal, err = r.readAmount(); err != nil {
		return nil, err
	}
	if prj.MemberCount, err = r.readUint64(); err != nil {
		return nil, err
	}
	if r.pos < len(r.data) {
		if prj.URL, err = r.readString(); err != nil {
			return nil, err
		}
	}
	return prj, nil
}

// EncodeProjectConfig serializes config structs so they can be cached outside the main project blob.
// Example payload: EncodeProjectConfig(&ProjectConfig{ThresholdPercent:55, MembershipNftPayloadFormat: "{nft}|{caller}"})
func EncodeProjectConfig(cfg *ProjectConfig) []byte {
	w := newWriter()
	encodeProjectConfig(w, cfg)
	return w.bytes()
}

// DecodeProjectConfig reverses the above encoding and is mostly used in tests/migration tools.
// Example payload: DecodeProjectConfig(EncodeProjectConfig(&ProjectConfig{ProposalCost:1.5}))
func DecodeProjectConfig(data []byte) (*ProjectConfig, error) {
	r := newReader(data)
	cfg, err := decodeProjectConfig(r)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// EncodeProjectMeta stores the light metadata slice so mutations dont touch finance bytes.
// Example payload: EncodeProjectMeta(&ProjectMeta{Name:"Club", Description:"all friends"})
func EncodeProjectMeta(meta *ProjectMeta) []byte {
	w := newWriter()
	w.writeAddress(meta.Owner)
	w.writeString(meta.Name)
	w.writeString(meta.Description)
	w.writeBool(meta.Paused)
	w.writeString(meta.Tx)
	w.writeString(meta.Metadata)
	w.writeString(meta.URL)
	return w.bytes()
}

// DecodeProjectMeta converts state bytes back into human fields for queries.
// Example payload: DecodeProjectMeta(EncodeProjectMeta(&ProjectMeta{Name:"test"}))
func DecodeProjectMeta(data []byte) (*ProjectMeta, error) {
	r := newReader(data)
	var meta ProjectMeta
	var err error
	if owner, err := r.readString(); err == nil {
		meta.Owner = AddressFromString(owner)
	} else {
		return nil, err
	}
	if meta.Name, err = r.readString(); err != nil {
		return nil, err
	}
	if meta.Description, err = r.readString(); err != nil {
		return nil, err
	}
	if meta.Paused, err = r.readBool(); err != nil {
		return nil, err
	}
	if meta.Tx, err = r.readString(); err != nil {
		return nil, err
	}
	if meta.Metadata, err = r.readString(); err != nil {
		return nil, err
	}
	if r.pos < len(r.data) {
		if meta.URL, err = r.readString(); err != nil {
			return nil, err
		}
	}
	return &meta, nil
}

// EncodeProjectFinance handles the frequently updated treasury counters separately.
// Example payload: EncodeProjectFinance(&ProjectFinance{MemberCount:3})
func EncodeProjectFinance(fin *ProjectFinance) []byte {
	w := newWriter()
	w.writeAsset(fin.FundsAsset)
	w.writeAmount(fin.StakeTotal)
	w.writeUint64(fin.MemberCount)
	return w.bytes()
}

// DecodeProjectFinance lets analytics read the compact finance snapshot.
// Example payload: DecodeProjectFinance(EncodeProjectFinance(&ProjectFinance{FundsAsset:AssetFromString("hive")}))
func DecodeProjectFinance(data []byte) (*ProjectFinance, error) {
	r := newReader(data)
	var fin ProjectFinance
	var err error
	if asset, err := r.readString(); err == nil {
		fin.FundsAsset = AssetFromString(asset)
	} else {
		return nil, err
	}
	if fin.StakeTotal, err = r.readAmount(); err != nil {
		return nil, err
	}
	if fin.MemberCount, err = r.readUint64(); err != nil {
		return nil, err
	}
	return &fin, nil
}

// DecodeProposal lets governance tooling inspect stored proposals with one helper call.
// Example payload: DecodeProposal(EncodeProposal(&Proposal{ID:11, ProjectID:2}))
func DecodeProposal(data []byte) (*Proposal, error) {
	r := newReader(data)
	prpsl := &Proposal{}
	var err error
	if prpsl.ID, err = r.readUint64(); err != nil {
		return nil, err
	}
	if prpsl.ProjectID, err = r.readUint64(); err != nil {
		return nil, err
	}
	if creator, err := r.readString(); err == nil {
		prpsl.Creator = AddressFromString(creator)
	} else {
		return nil, err
	}
	if prpsl.Name, err = r.readString(); err != nil {
		return nil, err
	}
	if prpsl.Description, err = r.readString(); err != nil {
		return nil, err
	}
	count, err := r.readUint64()
	if err != nil {
		return nil, err
	}
	prpsl.OptionCount = uint32(count)
	if prpsl.DurationHours, err = r.readUint64(); err != nil {
		return nil, err
	}
	if prpsl.CreatedAt, err = r.readInt64(); err != nil {
		return nil, err
	}
	stateByte, err := r.readByte()
	if err != nil {
		return nil, err
	}
	prpsl.State = ProposalState(stateByte)
	if prpsl.Outcome, err = decodeProposalOutcome(r); err != nil {
		return nil, err
	}
	if prpsl.Tx, err = r.readString(); err != nil {
		return nil, err
	}
	if prpsl.StakeSnapshot, err = r.readAmount(); err != nil {
		return nil, err
	}
	if count, err := r.readVarUint(); err == nil {
		prpsl.MemberCountSnapshot = uint(count)
	} else {
		return nil, err
	}
	if prpsl.Metadata, err = r.readString(); err != nil {
		return nil, err
	}
	if prpsl.IsPoll, err = r.readBool(); err != nil {
		return nil, err
	}
	if v, err := r.readVarInt(); err == nil {
		prpsl.ResultOptionID = int32(v)
	} else {
		return nil, err
	}
	if prpsl.ExecutableAt, err = r.readInt64(); err != nil {
		return nil, err
	}
	if r.pos < len(r.data) {
		if prpsl.URL, err = r.readString(); err != nil {
			return nil, err
		}
	}
	// Skip old voters list if present (backwards compatible)
	if r.pos < len(r.data) {
		voterCount, err := r.readVarUint()
		if err != nil {
			return nil, err
		}
		// Skip voter addresses
		for i := uint64(0); i < voterCount; i++ {
			_, err := r.readString()
			if err != nil {
				return nil, err
			}
		}
	}
	// Skip old total voted weight if present (backwards compatible)
	if r.pos < len(r.data) {
		_, err = r.readAmount()
		if err != nil {
			return nil, err
		}
	}
	return prpsl, nil
}

// DecodeCreateProjectArgs is mostly for integration tests that roundtrip payload encoding.
// Example payload: DecodeCreateProjectArgs(EncodeCreateProjectArgs(&CreateProjectArgs{Name:"coop"}))
func DecodeCreateProjectArgs(data []byte) (*CreateProjectArgs, error) {
	r := newReader(data)
	args := &CreateProjectArgs{}
	var err error
	if args.Name, err = r.readString(); err != nil {
		return nil, err
	}
	if args.Description, err = r.readString(); err != nil {
		return nil, err
	}
	if args.ProjectConfig, err = decodeProjectConfig(r); err != nil {
		return nil, err
	}
	if args.Metadata, err = r.readString(); err != nil {
		return nil, err
	}
	if r.pos < len(r.data) {
		if args.URL, err = r.readString(); err != nil {
			return nil, err
		}
	}
	return args, nil
}

// DecodeCreateProposalArgs follows the same idea but for proposal payloads.
// Example payload: DecodeCreateProposalArgs(EncodeCreateProposalArgs(&CreateProposalArgs{ProjectID:3}))
func DecodeCreateProposalArgs(data []byte) (*CreateProposalArgs, error) {
	r := newReader(data)
	args := &CreateProposalArgs{}
	var err error
	if args.ProjectID, err = r.readUint64(); err != nil {
		return nil, err
	}
	if args.Name, err = r.readString(); err != nil {
		return nil, err
	}
	if args.Description, err = r.readString(); err != nil {
		return nil, err
	}
	count, err := r.readVarUint()
	if err != nil {
		return nil, err
	}
	args.OptionsList = make([]ProposalOptionInput, count)
	for i := uint64(0); i < count; i++ {
		if args.OptionsList[i].Text, err = r.readString(); err != nil {
			return nil, err
		}
		if args.OptionsList[i].URL, err = r.readString(); err != nil {
			return nil, err
		}
	}
	if args.ProposalOutcome, err = decodeProposalOutcome(r); err != nil {
		return nil, err
	}
	if args.ProposalDuration, err = r.readUint64(); err != nil {
		return nil, err
	}
	if args.Metadata, err = r.readString(); err != nil {
		return nil, err
	}
	if r.pos < len(r.data) {
		if args.URL, err = r.readString(); err != nil {
			return nil, err
		}
	}
	return args, nil
}

// DecodeVoteProposalArgs decodes the wasm vote payload held in tests fixtures.
// Example payload: DecodeVoteProposalArgs(EncodeVoteProposalArgs(&VoteProposalArgs{ProposalID:4, Choices:[]uint{1}}))
func DecodeVoteProposalArgs(data []byte) (*VoteProposalArgs, error) {
	r := newReader(data)
	args := &VoteProposalArgs{}
	var err error
	if args.ProposalID, err = r.readUint64(); err != nil {
		return nil, err
	}
	count, err := r.readVarUint()
	if err != nil {
		return nil, err
	}
	args.Choices = make([]uint, count)
	for i := uint64(0); i < count; i++ {
		val, err := r.readVarUint()
		if err != nil {
			return nil, err
		}
		args.Choices[i] = uint(val)
	}
	return args, nil
}

// DecodeAddFundsArgs mirrors encode helper for funds payload validation.
// Example payload: DecodeAddFundsArgs(EncodeAddFundsArgs(&AddFundsArgs{ProjectID:1, ToStake:false}))
func DecodeAddFundsArgs(data []byte) (*AddFundsArgs, error) {
	r := newReader(data)
	args := &AddFundsArgs{}
	var err error
	if args.ProjectID, err = r.readUint64(); err != nil {
		return nil, err
	}
	if args.ToStake, err = r.readBool(); err != nil {
		return nil, err
	}
	return args, nil
}
