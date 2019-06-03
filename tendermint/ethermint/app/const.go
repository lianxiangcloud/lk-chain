package app

type SignerRoleType uint8

const (
	SignerRoleListener = SignerRoleType(0x01)
)

func (sr SignerRoleType) String() string {
		return "SignerRoleType"
}
