package crid

import "time"

// Parser decodes IDs without requiring a running Node.
type Parser struct {
	cfg  *config
	comF *compiledFormat
}

// NewParser creates a Parser configured with the given options.
func NewParser(opts ...Option) (*Parser, error) {
	cfg := applyOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	comF := cfg.format.compileFormat()
	return &Parser{
		cfg:  cfg,
		comF: comF,
	}, nil
}

// Parse decodes an ID into its timestamp and sequence components.
func (p *Parser) Parse(id ID) ParsedID {
	return parseWith(id, p.cfg.epoch, p.comF)
}

func parseWith(id ID, epoch time.Time, comF *compiledFormat) ParsedID {
	idI64 := int64(id)
	ts := (idI64 >> comF.shiftTimestamp) & comF.maxTimestamp
	seq := idI64 & comF.maxSequence
	return ParsedID{
		Timestamp: epoch.Add(time.Duration(ts) * time.Second),
		Sequence:  seq,
	}
}
