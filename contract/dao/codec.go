package dao

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"sort"
)

type binWriter struct {
	buf bytes.Buffer
}

func newWriter() *binWriter { return &binWriter{} }

func (w *binWriter) bytes() []byte { return w.buf.Bytes() }

func (w *binWriter) writeBool(v bool) {
	if v {
		w.buf.WriteByte(1)
	} else {
		w.buf.WriteByte(0)
	}
}

func (w *binWriter) writeUint64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	w.buf.Write(b[:])
}

func (w *binWriter) writeInt64(v int64) {
	w.writeUint64(uint64(v))
}

func (w *binWriter) writeFloat64(v float64) {
	w.writeUint64(math.Float64bits(v))
}

func (w *binWriter) writeVarUint(v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	w.buf.Write(tmp[:n])
}

func (w *binWriter) writeVarInt(v int64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutVarint(tmp[:], v)
	w.buf.Write(tmp[:n])
}

func (w *binWriter) writeAmount(v Amount) {
	w.writeInt64(int64(v))
}

func (w *binWriter) writeString(s string) {
	w.writeVarUint(uint64(len(s)))
	w.buf.WriteString(s)
}

func (w *binWriter) writeOptionalString(ptr *string) {
	if ptr == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeString(*ptr)
}

func (w *binWriter) writeOptionalUint64(ptr *uint64) {
	if ptr == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeUint64(*ptr)
}

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

func (w *binWriter) writeAddress(a Address) {
	w.writeString(addressString(a))
}

func (w *binWriter) writeAsset(a Asset) {
	w.writeString(assetString(a))
}

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
}

func encodeMember(w *binWriter, m *Member) {
	w.writeAddress(m.Address)
	w.writeAmount(m.Stake)
	w.writeInt64(m.JoinedAt)
	w.writeInt64(m.LastActionAt)
	w.writeInt64(m.ExitRequested)
	w.writeInt64(m.Reputation)
}

func EncodeMember(m *Member) []byte {
	w := newWriter()
	encodeMember(w, m)
	return w.bytes()
}

func encodeProposalOption(w *binWriter, opt *ProposalOption) {
	w.writeString(opt.Text)
	w.writeAmount(opt.WeightTotal)
	w.writeUint64(opt.VoterCount)
}

func encodeProposalOutcome(w *binWriter, out *ProposalOutcome) {
	if out == nil {
		w.writeBool(false)
		return
	}
	w.writeBool(true)
	w.writeStringMap(out.Meta)
	if out.Payout == nil {
		w.writeVarUint(0)
	} else {
		addresses := make([]string, 0, len(out.Payout))
		for addr := range out.Payout {
			addresses = append(addresses, addressString(addr))
		}
		sort.Strings(addresses)
		w.writeVarUint(uint64(len(addresses)))
		for _, addrStr := range addresses {
			w.writeString(addrStr)
			w.writeAmount(out.Payout[newAddress(addrStr)])
		}
	}
}

// EncodeProject serializes a project to a compact binary form.
func EncodeProject(prj *Project) []byte {
	w := newWriter()
	w.writeUint64(prj.ID)
	w.writeAddress(prj.Owner)
	w.writeString(prj.Name)
	w.writeString(prj.Description)
	encodeProjectConfig(w, &prj.Config)
	w.writeAmount(prj.Funds)
	w.writeAsset(prj.FundsAsset)
	w.writeBool(prj.Paused)
	w.writeString(prj.Tx)
	w.writeString(prj.Metadata)
	w.writeAmount(prj.StakeTotal)
	w.writeUint64(prj.MemberCount)
	return w.bytes()
}

// EncodeProposal serializes a proposal.
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
	return w.bytes()
}

func EncodeCreateProjectArgs(args *CreateProjectArgs) []byte {
	w := newWriter()
	w.writeString(args.Name)
	w.writeString(args.Description)
	encodeProjectConfig(w, &args.ProjectConfig)
	w.writeString(args.Metadata)
	return w.bytes()
}

func EncodeCreateProposalArgs(args *CreateProposalArgs) []byte {
	w := newWriter()
	w.writeUint64(args.ProjectID)
	w.writeString(args.Name)
	w.writeString(args.Description)
	w.writeVarUint(uint64(len(args.OptionsList)))
	for _, opt := range args.OptionsList {
		w.writeString(opt)
	}
	encodeProposalOutcome(w, args.ProposalOutcome)
	w.writeUint64(args.ProposalDuration)
	w.writeString(args.Metadata)
	return w.bytes()
}

func EncodeVoteProposalArgs(args *VoteProposalArgs) []byte {
	w := newWriter()
	w.writeUint64(args.ProposalID)
	w.writeVarUint(uint64(len(args.Choices)))
	for _, choice := range args.Choices {
		w.writeVarUint(uint64(choice))
	}
	return w.bytes()
}

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

func newReader(data []byte) *binReader {
	return &binReader{data: data}
}

func (r *binReader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("unexpected EOF")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *binReader) readBool() (bool, error) {
	b, err := r.readByte()
	if err != nil {
		return false, err
	}
	return b == 1, nil
}

func (r *binReader) readUint64() (uint64, error) {
	if r.pos+8 > len(r.data) {
		return 0, errors.New("unexpected EOF")
	}
	val := binary.BigEndian.Uint64(r.data[r.pos : r.pos+8])
	r.pos += 8
	return val, nil
}

func (r *binReader) readInt64() (int64, error) {
	v, err := r.readUint64()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

func (r *binReader) readFloat64() (float64, error) {
	v, err := r.readUint64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

func (r *binReader) readVarUint() (uint64, error) {
	val, n := binary.Uvarint(r.data[r.pos:])
	if n <= 0 {
		return 0, errors.New("invalid varuint")
	}
	r.pos += n
	return val, nil
}

func (r *binReader) readVarInt() (int64, error) {
	val, n := binary.Varint(r.data[r.pos:])
	if n <= 0 {
		return 0, errors.New("invalid varint")
	}
	r.pos += n
	return val, nil
}

func (r *binReader) readAmount() (Amount, error) {
	val, err := r.readInt64()
	if err != nil {
		return 0, err
	}
	return Amount(val), nil
}

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
	return cfg, nil
}

func decodeMember(r *binReader) (Member, error) {
	var m Member
	addr, err := r.readString()
	if err != nil {
		return m, err
	}
	m.Address = newAddress(addr)
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
	return m, nil
}

func DecodeMember(data []byte) (*Member, error) {
	r := newReader(data)
	m, err := decodeMember(r)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func decodeProposalOption(r *binReader) (ProposalOption, error) {
	var opt ProposalOption
	var err error
	if opt.Text, err = r.readString(); err != nil {
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

func EncodeProposalOption(opt *ProposalOption) []byte {
	w := newWriter()
	encodeProposalOption(w, opt)
	return w.bytes()
}

func DecodeProposalOption(data []byte) (*ProposalOption, error) {
	r := newReader(data)
	opt, err := decodeProposalOption(r)
	if err != nil {
		return nil, err
	}
	return &opt, nil
}

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
	payouts := make(map[Address]Amount, count)
	for i := uint64(0); i < count; i++ {
		addr, err := r.readString()
		if err != nil {
			return nil, err
		}
		value, err := r.readAmount()
		if err != nil {
			return nil, err
		}
		payouts[newAddress(addr)] = value
	}
	return &ProposalOutcome{
		Meta:   meta,
		Payout: payouts,
	}, nil
}

func DecodeProject(data []byte) (*Project, error) {
	r := newReader(data)
	prj := &Project{}
	var err error
	if prj.ID, err = r.readUint64(); err != nil {
		return nil, err
	}
	if owner, err := r.readString(); err == nil {
		prj.Owner = newAddress(owner)
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
	if prj.Funds, err = r.readAmount(); err != nil {
		return nil, err
	}
	if asset, err := r.readString(); err == nil {
		prj.FundsAsset = newAsset(asset)
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
	return prj, nil
}

// EncodeProjectConfig serializes a project configuration block.
func EncodeProjectConfig(cfg *ProjectConfig) []byte {
	w := newWriter()
	encodeProjectConfig(w, cfg)
	return w.bytes()
}

// DecodeProjectConfig deserializes a project configuration.
func DecodeProjectConfig(data []byte) (*ProjectConfig, error) {
	r := newReader(data)
	cfg, err := decodeProjectConfig(r)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// EncodeProjectMeta serializes project metadata.
func EncodeProjectMeta(meta *ProjectMeta) []byte {
	w := newWriter()
	w.writeAddress(meta.Owner)
	w.writeString(meta.Name)
	w.writeString(meta.Description)
	w.writeBool(meta.Paused)
	w.writeString(meta.Tx)
	w.writeString(meta.Metadata)
	return w.bytes()
}

// DecodeProjectMeta deserializes project metadata.
func DecodeProjectMeta(data []byte) (*ProjectMeta, error) {
	r := newReader(data)
	var meta ProjectMeta
	var err error
	if owner, err := r.readString(); err == nil {
		meta.Owner = newAddress(owner)
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
	return &meta, nil
}

// EncodeProjectFinance serializes treasury/staking aggregates.
func EncodeProjectFinance(fin *ProjectFinance) []byte {
	w := newWriter()
	w.writeAmount(fin.Funds)
	w.writeAsset(fin.FundsAsset)
	w.writeAmount(fin.StakeTotal)
	w.writeUint64(fin.MemberCount)
	return w.bytes()
}

// DecodeProjectFinance deserializes finance data.
func DecodeProjectFinance(data []byte) (*ProjectFinance, error) {
	r := newReader(data)
	var fin ProjectFinance
	var err error
	if fin.Funds, err = r.readAmount(); err != nil {
		return nil, err
	}
	if asset, err := r.readString(); err == nil {
		fin.FundsAsset = newAsset(asset)
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
		prpsl.Creator = newAddress(creator)
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
	return prpsl, nil
}

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
	return args, nil
}

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
	args.OptionsList = make([]string, count)
	for i := uint64(0); i < count; i++ {
		if args.OptionsList[i], err = r.readString(); err != nil {
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
	return args, nil
}

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
