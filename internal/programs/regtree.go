package programs

import (
	"errors"
	"strings"

	"axion/internal/sys"
)

// RegTree is a registry key with its values and sub-keys, as saved in the
// quarantine manifest so a removed key can be written back exactly.
type RegTree struct {
	Key    string                  `json:"key"`
	Values map[string]sys.RegValue `json:"values,omitempty"`
	Subs   []*RegTree              `json:"subs,omitempty"`
}

const (
	maxTreeDepth = 32
	maxTreeNodes = 20000
)

var errTreeTooBig = errors.New("chave do Registro grande demais para a quarentena")

// ReadTree copies a key and everything below it.
func ReadTree(r sys.Registry, key string) (*RegTree, error) {
	n := 0
	return readTree(r, key, 0, &n)
}

func readTree(r sys.Registry, key string, depth int, n *int) (*RegTree, error) {
	*n++
	if depth > maxTreeDepth || *n > maxTreeNodes {
		return nil, errTreeTooBig
	}
	t := &RegTree{Key: key, Values: map[string]sys.RegValue{}}
	names, err := r.ValueNames(key)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		v, err := r.GetValue(key, name)
		if err != nil {
			return nil, err
		}
		if v.Exists {
			t.Values[name] = v
		}
	}
	subs, err := r.SubKeys(key)
	if err != nil {
		return nil, err
	}
	for _, s := range subs {
		c, err := readTree(r, key+`\`+s, depth+1, n)
		if err != nil {
			return nil, err
		}
		t.Subs = append(t.Subs, c)
	}
	return t, nil
}

// WriteTree recreates a saved key (existing values are overwritten).
func WriteTree(r sys.Registry, t *RegTree) error {
	if t == nil {
		return nil
	}
	if _, err := r.CreateKey(t.Key); err != nil {
		return err
	}
	for name, v := range t.Values {
		if err := r.SetValue(t.Key, name, v); err != nil {
			return err
		}
	}
	for _, s := range t.Subs {
		if !strings.HasPrefix(strings.ToLower(s.Key), strings.ToLower(t.Key)+`\`) {
			return errors.New("manifesto da quarentena inconsistente")
		}
		if err := WriteTree(r, s); err != nil {
			return err
		}
	}
	return nil
}

// DeleteTree removes a saved key bottom-up (values, then the empty key).
func DeleteTree(r sys.Registry, t *RegTree) error {
	if t == nil {
		return nil
	}
	for i := len(t.Subs) - 1; i >= 0; i-- {
		if err := DeleteTree(r, t.Subs[i]); err != nil {
			return err
		}
	}
	for name := range t.Values {
		if err := r.DeleteValue(t.Key, name); err != nil {
			return err
		}
	}
	if ok, err := r.DeleteKeyIfEmpty(t.Key); err != nil {
		return err
	} else if !ok {
		if exists, _ := r.KeyExists(t.Key); exists {
			return errors.New("a chave " + t.Key + " ganhou conteúdo novo e não foi removida")
		}
	}
	return nil
}
