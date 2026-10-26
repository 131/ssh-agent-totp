package update

import (
	"bytes"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/131/ssh-agent-totp/internal/i18n"
)

var (
	crypt32              = windows.NewLazySystemDLL("crypt32.dll")
	procCryptMsgGetParam = crypt32.NewProc("CryptMsgGetParam")
	procCryptMsgClose    = crypt32.NewProc("CryptMsgClose")
)

const cmsgSignerCertInfoParam = 7

// Authenticode accepts candidate only when Windows trusts its Authenticode
// signature (whole chain, revocation included) and its signer has the same
// subject as the signer of the running executable. An unsigned running build
// cannot update itself.
func Authenticode(candidate, current string) error {
	running, _, err := signer(current)
	if err != nil {
		return i18n.Errorf("err.unsigned", err)
	}
	if err := trusted(candidate); err != nil {
		return i18n.Errorf("err.signature", err)
	}
	subject, name, err := signer(candidate)
	if err != nil {
		return i18n.Errorf("err.signer.read", err)
	}
	if !bytes.Equal(subject, running) {
		return i18n.Errorf("err.signer", name)
	}
	return nil
}

func trusted(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	file := windows.WinTrustFileInfo{FilePath: name}
	file.Size = uint32(unsafe.Sizeof(file))
	data := windows.WinTrustData{
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_WHOLECHAIN,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file),
	}
	data.Size = uint32(unsafe.Sizeof(data))
	verifyErr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	return verifyErr
}

// signer returns the encoded subject and display name of the certificate that
// signed path.
func signer(path string) ([]byte, string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, "", err
	}
	var encoding, content, format uint32
	var store, msg windows.Handle
	err = windows.CryptQueryObject(windows.CERT_QUERY_OBJECT_FILE, unsafe.Pointer(name),
		windows.CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED, windows.CERT_QUERY_FORMAT_FLAG_BINARY, 0,
		&encoding, &content, &format, &store, &msg, nil)
	if err != nil {
		return nil, "", err
	}
	defer windows.CertCloseStore(store, 0)
	defer procCryptMsgClose.Call(uintptr(msg))

	var size uint32
	if ok, _, err := procCryptMsgGetParam.Call(uintptr(msg), cmsgSignerCertInfoParam, 0, 0, uintptr(unsafe.Pointer(&size))); ok == 0 {
		return nil, "", err
	}
	if size < uint32(unsafe.Sizeof(windows.CertInfo{})) {
		return nil, "", i18n.Errorf("err.signer.missing")
	}
	// CertInfo holds pointers into this buffer; allocate it as words to keep
	// the structure aligned.
	buffer := make([]uint64, (size+7)/8)
	if ok, _, err := procCryptMsgGetParam.Call(uintptr(msg), cmsgSignerCertInfoParam, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size))); ok == 0 {
		return nil, "", err
	}
	cert, err := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, 0,
		windows.CERT_FIND_SUBJECT_CERT, unsafe.Pointer(&buffer[0]), nil)
	if err != nil {
		return nil, "", err
	}
	defer windows.CertFreeCertificateContext(cert)
	blob := cert.CertInfo.Subject
	subject := bytes.Clone(unsafe.Slice(blob.Data, blob.Size))
	display := make([]uint16, 256)
	windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &display[0], uint32(len(display)))
	return subject, windows.UTF16ToString(display), nil
}
