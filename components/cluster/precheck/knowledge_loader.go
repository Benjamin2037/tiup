package precheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// KnowledgeBaseDir 默认本地知识库目录，可通过环境变量覆盖
var KnowledgeBaseDir = getEnvOrDefault("TIUP_KNOWLEDGE_DIR", "./knowledge")

// RemoteKnowledgeBaseURL 官方知识库包下载地址前缀（可根据实际情况调整）
var RemoteKnowledgeBaseURL = getEnvOrDefault("TIUP_KNOWLEDGE_REMOTE", "https://download.pingcap.com/knowledge/")

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// CheckAndLoadDefaults 检查并加载指定版本的 defaults.json
func CheckAndLoadDefaults(version string) (map[string]string, error) {
	path := filepath.Join(KnowledgeBaseDir, version, "defaults.json")
	if !fileExists(path) {
		if err := DownloadDefaultsFromRemote(version); err != nil {
			return nil, fmt.Errorf("defaults.json 不存在且远程下载失败: %w", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 defaults.json 失败: %w", err)
	}
	defer f.Close()
	var defaults map[string]string
	if err := json.NewDecoder(f).Decode(&defaults); err != nil {
		return nil, fmt.Errorf("解析 defaults.json 失败: %w", err)
	}
	return defaults, nil
}

// CheckAndLoadUpgradeLogic 检查并加载 upgrade_logic.json
func CheckAndLoadUpgradeLogic() (map[string]interface{}, error) {
	path := filepath.Join(KnowledgeBaseDir, "upgrade_logic.json")
	if !fileExists(path) {
		if err := DownloadUpgradeLogicFromRemote(); err != nil {
			return nil, fmt.Errorf("upgrade_logic.json 不存在且远程下载失败: %w", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 upgrade_logic.json 失败: %w", err)
	}
	defer f.Close()
	var logic map[string]interface{}
	if err := json.NewDecoder(f).Decode(&logic); err != nil {
		return nil, fmt.Errorf("解析 upgrade_logic.json 失败: %w", err)
	}
	return logic, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DownloadDefaultsFromRemote 从远程拉取 defaults.json
func DownloadDefaultsFromRemote(version string) error {
	url := fmt.Sprintf("%s%s/defaults.json", RemoteKnowledgeBaseURL, version)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载失败: %s", resp.Status)
	}
	localDir := filepath.Join(KnowledgeBaseDir, version)
	os.MkdirAll(localDir, 0755)
	localPath := filepath.Join(localDir, "defaults.json")
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// DownloadUpgradeLogicFromRemote 从远程拉取 upgrade_logic.json
func DownloadUpgradeLogicFromRemote() error {
	url := fmt.Sprintf("%supgrade_logic.json", RemoteKnowledgeBaseURL)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载失败: %s", resp.Status)
	}
	os.MkdirAll(KnowledgeBaseDir, 0755)
	localPath := filepath.Join(KnowledgeBaseDir, "upgrade_logic.json")
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
