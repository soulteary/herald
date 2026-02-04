package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	// Test with empty directory (uses built-in templates)
	manager := NewManager("")
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.dir != "" {
		t.Errorf("NewManager() dir = %v, want empty string", manager.dir)
	}
	if manager.templates == nil {
		t.Error("NewManager() templates map is nil")
	}

	// Test with non-existent directory (should still work, uses built-in)
	manager2 := NewManager("/nonexistent/dir")
	if manager2 == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestManager_Render_BuiltIn(t *testing.T) {
	manager := NewManager("")

	tests := []struct {
		name     string
		locale   string
		channel  string
		purpose  string
		data     TemplateData
		wantErr  bool
		contains string
	}{
		{
			name:    "email channel, English locale",
			locale:  "en",
			channel: "email",
			purpose: "login",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "en",
			},
			wantErr:  false,
			contains: "123456",
		},
		{
			name:    "email channel, Chinese locale",
			locale:  "zh-CN",
			channel: "email",
			purpose: "login",
			data: TemplateData{
				Code:      "654321",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "zh-CN",
			},
			wantErr:  false,
			contains: "654321",
		},
		{
			name:    "sms channel, English locale",
			locale:  "en",
			channel: "sms",
			purpose: "login",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "en",
			},
			wantErr:  false,
			contains: "123456",
		},
		{
			name:    "sms channel, Chinese locale",
			locale:  "zh-CN",
			channel: "sms",
			purpose: "login",
			data: TemplateData{
				Code:      "654321",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "zh-CN",
			},
			wantErr:  false,
			contains: "654321",
		},
		{
			name:    "unsupported channel",
			locale:  "en",
			channel: "invalid",
			purpose: "login",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.Render(tt.locale, tt.channel, tt.purpose, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if result == "" {
					t.Error("Render() returned empty string")
				}
				if tt.contains != "" && !contains(result, tt.contains) {
					t.Errorf("Render() result = %v, should contain %v", result, tt.contains)
				}
			}
		})
	}
}

func TestManager_RenderEmail(t *testing.T) {
	manager := NewManager("")

	tests := []struct {
		name     string
		locale   string
		purpose  string
		data     TemplateData
		contains string
	}{
		{
			name:    "English locale, login purpose",
			locale:  "en",
			purpose: "login",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "en",
			},
			contains: "123456",
		},
		{
			name:    "Chinese locale, login purpose",
			locale:  "zh-CN",
			purpose: "login",
			data: TemplateData{
				Code:      "654321",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "zh-CN",
			},
			contains: "654321",
		},
		{
			name:    "English locale, reset purpose",
			locale:  "en",
			purpose: "reset",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   "reset",
				Locale:    "en",
			},
			contains: "123456",
		},
		{
			name:    "Chinese locale, reset purpose",
			locale:  "zh-CN",
			purpose: "reset",
			data: TemplateData{
				Code:      "654321",
				ExpiresIn: 300,
				Purpose:   "reset",
				Locale:    "zh-CN",
			},
			contains: "654321",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, body, err := manager.RenderEmail(tt.locale, tt.purpose, tt.data)
			if err != nil {
				t.Errorf("RenderEmail() error = %v", err)
				return
			}
			if subject == "" {
				t.Error("RenderEmail() subject is empty")
			}
			if body == "" {
				t.Error("RenderEmail() body is empty")
			}
			if tt.contains != "" && !contains(body, tt.contains) {
				t.Errorf("RenderEmail() body = %v, should contain %v", body, tt.contains)
			}
		})
	}
}

func TestManager_RenderSMS(t *testing.T) {
	manager := NewManager("")

	tests := []struct {
		name     string
		locale   string
		purpose  string
		data     TemplateData
		contains string
	}{
		{
			name:    "English locale, login purpose",
			locale:  "en",
			purpose: "login",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "en",
			},
			contains: "123456",
		},
		{
			name:    "Chinese locale, login purpose",
			locale:  "zh-CN",
			purpose: "login",
			data: TemplateData{
				Code:      "654321",
				ExpiresIn: 300,
				Purpose:   "login",
				Locale:    "zh-CN",
			},
			contains: "654321",
		},
		{
			name:    "English locale, reset purpose",
			locale:  "en",
			purpose: "reset",
			data: TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   "reset",
				Locale:    "en",
			},
			contains: "123456",
		},
		{
			name:    "Chinese locale, reset purpose",
			locale:  "zh-CN",
			purpose: "reset",
			data: TemplateData{
				Code:      "654321",
				ExpiresIn: 300,
				Purpose:   "reset",
				Locale:    "zh-CN",
			},
			contains: "654321",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := manager.RenderSMS(tt.locale, tt.purpose, tt.data)
			if err != nil {
				t.Errorf("RenderSMS() error = %v", err)
				return
			}
			if body == "" {
				t.Error("RenderSMS() body is empty")
			}
			if tt.contains != "" && !contains(body, tt.contains) {
				t.Errorf("RenderSMS() body = %v, should contain %v", body, tt.contains)
			}
		})
	}
}

func TestManager_Render_WithTemplateFiles(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	localeDir := filepath.Join(tmpDir, "en")
	emailDir := filepath.Join(localeDir, "email")
	if err := os.MkdirAll(emailDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create a template file
	templateFile := filepath.Join(emailDir, "login.txt")
	templateContent := "Subject: Test Verification\n\nYour code is: {{.Code}}"
	if err := os.WriteFile(templateFile, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	manager := NewManager(tmpDir)

	data := TemplateData{
		Code:      "123456",
		ExpiresIn: 300,
		Purpose:   "login",
		Locale:    "en",
	}

	// Test Render with custom template
	result, err := manager.Render("en", "email", "login", data)
	if err != nil {
		t.Errorf("Render() error = %v", err)
	}
	if result == "" {
		t.Error("Render() returned empty string")
	}
	if !contains(result, "123456") {
		t.Errorf("Render() result = %v, should contain 123456", result)
	}

	// Test RenderEmail with custom template
	subject, body, err := manager.RenderEmail("en", "login", data)
	if err != nil {
		t.Errorf("RenderEmail() error = %v", err)
	}
	if subject == "" {
		t.Error("RenderEmail() subject is empty")
	}
	if body == "" {
		t.Error("RenderEmail() body is empty")
	}
}

func TestGetPurposeName(t *testing.T) {
	tests := []struct {
		name     string
		locale   string
		purpose  string
		expected string
	}{
		{
			name:     "English, login",
			locale:   "en",
			purpose:  "login",
			expected: "login",
		},
		{
			name:     "English, reset",
			locale:   "en",
			purpose:  "reset",
			expected: "password reset",
		},
		{
			name:     "English, bind",
			locale:   "en",
			purpose:  "bind",
			expected: "binding",
		},
		{
			name:     "English, stepup",
			locale:   "en",
			purpose:  "stepup",
			expected: "step-up authentication",
		},
		{
			name:     "Chinese, login",
			locale:   "zh-CN",
			purpose:  "login",
			expected: "登录",
		},
		{
			name:     "Chinese, reset",
			locale:   "zh-CN",
			purpose:  "reset",
			expected: "重置密码",
		},
		{
			name:     "Chinese, bind",
			locale:   "zh-CN",
			purpose:  "bind",
			expected: "绑定",
		},
		{
			name:     "Chinese, stepup",
			locale:   "zh-CN",
			purpose:  "stepup",
			expected: "二次验证",
		},
		{
			name:     "Chinese prefix, login",
			locale:   "zh",
			purpose:  "login",
			expected: "登录",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test through RenderEmail to verify getPurposeName is called correctly
			manager := NewManager("")
			data := TemplateData{
				Code:      "123456",
				ExpiresIn: 300,
				Purpose:   tt.purpose,
				Locale:    tt.locale,
			}

			_, body, err := manager.RenderEmail(tt.locale, tt.purpose, data)
			if err != nil {
				t.Errorf("RenderEmail() error = %v", err)
				return
			}

			// For non-login purposes, verify the purpose name appears in the body
			if tt.purpose != "login" {
				if !contains(body, tt.expected) {
					t.Errorf("RenderEmail() body = %v, should contain purpose name %v", body, tt.expected)
				}
			}
		})
	}
}

func TestManager_GetPurposeName_Direct(t *testing.T) {
	manager := NewManager("")
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	// Direct call to GetPurposeName
	got := manager.GetPurposeName("en", "login")
	if got != "login" {
		t.Errorf("GetPurposeName(en, login) = %q, want login", got)
	}
	got = manager.GetPurposeName("zh-CN", "reset")
	if got != "重置密码" {
		t.Errorf("GetPurposeName(zh-CN, reset) = %q, want 重置密码", got)
	}
}

func TestManager_GetBundle_Direct(t *testing.T) {
	manager := NewManager("")
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	bundle := manager.GetBundle()
	if bundle == nil {
		t.Error("GetBundle() returned nil")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
