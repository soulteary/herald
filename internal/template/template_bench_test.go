package template

import "testing"

func BenchmarkRenderSMS(b *testing.B) {
	m := NewManager("")
	data := TemplateData{Code: "123456", ExpiresIn: 300}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := m.RenderSMS("en", "login", data); err != nil {
			b.Fatal(err)
		}
	}
}
