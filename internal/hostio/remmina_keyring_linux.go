package hostio

import ss "github.com/zalando/go-keyring/secret_service"

// systemRemminaKeyring looks up Remmina's passwords through the Secret
// Service (GNOME Keyring, KWallet). One session serves the whole scan, so a
// locked keyring asks to be unlocked once at most.
func systemRemminaKeyring() (lookup func(file string) string, done func()) {
	none := func(string) string { return "" }
	svc, err := ss.NewSecretService()
	if err != nil {
		return none, func() {}
	}
	collection := svc.GetLoginCollection()
	if err := svc.Unlock(collection.Path()); err != nil {
		return none, func() {}
	}
	session, err := svc.OpenSession()
	if err != nil {
		return none, func() {}
	}
	lookup = func(file string) string {
		items, err := svc.SearchItems(collection, map[string]string{"filename": file, "key": "password"})
		if err != nil || len(items) == 0 {
			return ""
		}
		item := items[0]
		if err := svc.Unlock(item); err != nil {
			return ""
		}
		s, err := svc.GetSecret(item, session.Path())
		if err != nil {
			return ""
		}
		return string(s.Value)
	}
	return lookup, func() { _ = svc.Close(session) }
}
