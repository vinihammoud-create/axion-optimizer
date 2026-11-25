package games

import "testing"

func FuzzParseVDF(f *testing.F) {
	f.Add("// comment\n\"a\" { \"b\" \"x\\\\y\" \"c\" [$WIN32] { \"d\" \"1\" } }")
	f.Add(`"a" { "b" `)
	f.Add(realVideo)
	f.Fuzz(func(t *testing.T, src string) {
		ParseVDF(src)
	})
}

func FuzzParseConvars(f *testing.F) {
	f.Add("\"config\"\n{\n\t\"convars\"\n\t{\n\t\t\"cl_net_buffer_ticks\"\t\t\"2\"\n\t\t\"rate\"\t\t\"196608\"\n\t\t\"sensitivity\"\t\t\"1.25\"\n\t}\n}\n")
	f.Add(`"outro" { "x" "1" }`)
	f.Fuzz(func(t *testing.T, src string) {
		ParseConvars(src)
	})
}

func FuzzParseCS2Video(f *testing.F) {
	f.Add(realVideo)
	f.Add("")
	f.Fuzz(func(t *testing.T, src string) {
		ParseCS2Video(src)
	})
}
