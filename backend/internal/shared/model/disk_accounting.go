package model

// DiskAccounting identifies the units behind a disk measurement, independently
// of whether the requested allocation is currently enforced. Empty is the legacy
// logical-file-size measurement, not an unknown or zero reading.
type DiskAccounting string

// Accounting methods distinguish filesystem charges from logical file sizes.
const (
	DiskAccountingLogical DiskAccounting = "logical"
	DiskAccountingProject DiskAccounting = "project"
	DiskAccountingQgroup  DiskAccounting = "qgroup"
	DiskAccountingMixed   DiskAccounting = "mixed"
)

// Valid accepts known methods and the omitted legacy field.
func (a DiskAccounting) Valid() bool {
	return a == "" || a == DiskAccountingLogical || a == DiskAccountingProject || a == DiskAccountingQgroup || a == DiskAccountingMixed
}

// Effective interprets older Sparks' omitted method as logical file sizes.
func (a DiskAccounting) Effective() DiskAccounting {
	if a == "" {
		return DiskAccountingLogical
	}

	return a
}
