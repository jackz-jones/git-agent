package conflict

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConflictType 冲突类型
type ConflictType string

const (
	ConflictEditEdit   ConflictType = "edit_edit"   // 双方修改了同一文件的同一部分
	ConflictEditDelete ConflictType = "edit_delete" // 一方修改另一方删除
	ConflictAddAdd     ConflictType = "add_add"     // 双方添加了同名文件
)

// FileConflict 表示一个文件冲突
type FileConflict struct {
	FilePath    string       `json:"file_path"`
	ConflictType ConflictType `json:"conflict_type"`
	OurChange   string       `json:"our_change"`   // 我们的修改描述
	TheirChange string       `json:"their_change"` // 对方的修改描述
	AutoResolvable bool      `json:"auto_resolvable"` // 是否可自动解决
	Resolution  *ResolutionSuggestion `json:"resolution,omitempty"`
}

// ResolutionSuggestion 解决建议
type ResolutionSuggestion struct {
	Strategy   string `json:"strategy"`    // ours, theirs, merge, manual
	Reason     string `json:"reason"`
	Confidence float64 `json:"confidence"` // 建议置信度
}

// ResolvedConflict 已解决的冲突
type ResolvedConflict struct {
	FilePath  string `json:"file_path"`
	Strategy  string `json:"strategy"`
	Result    string `json:"result"` // 解决结果描述
}

// Detector 冲突检测器
type Detector struct {
	repoPath string
}

// NewDetector 创建冲突检测器
func NewDetector(repoPath string) *Detector {
	return &Detector{
		repoPath: repoPath,
	}
}

// Scan 扫描仓库中的冲突
func (d *Detector) Scan() ([]FileConflict, error) {
	var conflicts []FileConflict

	// 扫描工作目录中包含冲突标记的文件
	err := filepath.Walk(d.repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// 跳过 .git 目录
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		// 读取文件内容检查冲突标记
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		contentStr := string(content)
		if strings.Contains(contentStr, "<<<<<<< ") || strings.Contains(contentStr, ">>>>>>> ") {
			relPath, _ := filepath.Rel(d.repoPath, path)

			conflict := FileConflict{
				FilePath:    relPath,
				ConflictType: ConflictEditEdit,
				AutoResolvable: d.canAutoResolve(contentStr),
			}

			// 提取冲突区域描述
			conflict.OurChange = d.extractOurChange(contentStr)
			conflict.TheirChange = d.extractTheirChange(contentStr)

			// 生成解决建议
			conflict.Resolution = d.suggestResolution(conflict)

			conflicts = append(conflicts, conflict)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("扫描冲突失败: %w", err)
	}

	return conflicts, nil
}

// Resolve 解决指定冲突
func (d *Detector) Resolve(conflictID string, strategy string) (*ResolvedConflict, error) {
	// conflictID 这里是文件路径
	filePath := filepath.Join(d.repoPath, conflictID)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	contentStr := string(content)

	var resolvedContent string
	var resultDesc string

	switch strategy {
	case "ours":
		resolvedContent = d.resolveWithOurs(contentStr)
		resultDesc = "已保留您的修改"
	case "theirs":
		resolvedContent = d.resolveWithTheirs(contentStr)
		resultDesc = "已采用对方的修改"
	case "merge":
		resolvedContent = d.resolveWithMerge(contentStr)
		resultDesc = "已自动合并双方修改"
	default:
		return nil, fmt.Errorf("未知的解决策略: %s（可选: ours, theirs, merge）", strategy)
	}

	// 写回文件
	err = os.WriteFile(filePath, []byte(resolvedContent), 0644)
	if err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	return &ResolvedConflict{
		FilePath: conflictID,
		Strategy: strategy,
		Result:   resultDesc,
	}, nil
}

// SuggestResolution 为冲突提供解决建议
func (d *Detector) SuggestResolution(conflict FileConflict) *ResolutionSuggestion {
	return d.suggestResolution(conflict)
}

// AutoResolveSimpleConflicts 自动解决简单冲突
func (d *Detector) AutoResolveSimpleConflicts() ([]ResolvedConflict, error) {
	conflicts, err := d.Scan()
	if err != nil {
		return nil, err
	}

	var resolved []ResolvedConflict
	for _, c := range conflicts {
		if !c.AutoResolvable {
			continue
		}

		strategy := "merge"
		if c.Resolution != nil {
			strategy = c.Resolution.Strategy
		}

		result, err := d.Resolve(c.FilePath, strategy)
		if err != nil {
			continue
		}
		resolved = append(resolved, *result)
	}

	return resolved, nil
}

// canAutoResolve 判断冲突是否可以自动解决
func (d *Detector) canAutoResolve(content string) bool {
	// 简单策略：如果冲突区域不重叠或只有一方修改，可以自动解决
	sections := d.countConflictSections(content)
	if sections == 1 {
		// 单个冲突区域，更有可能自动解决
		return true
	}
	return sections <= 2
}

// countConflictSections 计算冲突区域数量
func (d *Detector) countConflictSections(content string) int {
	return strings.Count(content, "<<<<<<< ")
}

// extractOurChange 提取我们的修改描述
func (d *Detector) extractOurChange(content string) string {
	parts := strings.SplitN(content, "<<<<<<< ", 2)
	if len(parts) < 2 {
		return ""
	}
	afterMarker := parts[1]
	endIdx := strings.Index(afterMarker, "=======")
	if endIdx < 0 {
		return ""
	}
	ourPart := strings.TrimSpace(afterMarker[:endIdx])
	if len(ourPart) > 100 {
		return ourPart[:100] + "..."
	}
	return ourPart
}

// extractTheirChange 提取对方的修改描述
func (d *Detector) extractTheirChange(content string) string {
	parts := strings.SplitN(content, "=======", 2)
	if len(parts) < 2 {
		return ""
	}
	afterMarker := parts[1]
	startIdx := strings.Index(afterMarker, ">>>>>>> ")
	if startIdx < 0 {
		return ""
	}
	theirPart := strings.TrimSpace(afterMarker[:startIdx])
	if len(theirPart) > 100 {
		return theirPart[:100] + "..."
	}
	return theirPart
}

// suggestResolution 生成解决建议
func (d *Detector) suggestResolution(conflict FileConflict) *ResolutionSuggestion {
	if conflict.AutoResolvable {
		return &ResolutionSuggestion{
			Strategy:   "merge",
			Reason:     "冲突区域简单，建议自动合并",
			Confidence: 0.8,
		}
	}

	// 如果一方修改很少，建议保留另一方的
	if len(conflict.OurChange) < 20 {
		return &ResolutionSuggestion{
			Strategy:   "theirs",
			Reason:     "您的修改较少，建议采用对方的版本",
			Confidence: 0.6,
		}
	}
	if len(conflict.TheirChange) < 20 {
		return &ResolutionSuggestion{
			Strategy:   "ours",
			Reason:     "对方修改较少，建议保留您的版本",
			Confidence: 0.6,
		}
	}

	return &ResolutionSuggestion{
		Strategy:   "manual",
		Reason:     "冲突较复杂，建议手动确认",
		Confidence: 0.3,
	}
}

// resolveWithOurs 保留我们的修改
func (d *Detector) resolveWithOurs(content string) string {
	return d.resolveConflict(content, "ours")
}

// resolveWithTheirs 保留对方的修改
func (d *Detector) resolveWithTheirs(content string) string {
	return d.resolveConflict(content, "theirs")
}

// resolveWithMerge 尝试自动合并
func (d *Detector) resolveWithMerge(content string) string {
	return d.resolveConflict(content, "ours") // 简化实现：默认采用 ours，后续可增强
}

// resolveConflict 通用冲突解决
func (d *Detector) resolveConflict(content, strategy string) string {
	var result strings.Builder
	remaining := content

	for strings.Contains(remaining, "<<<<<<< ") {
		// 找到冲突标记之前的文本
		conflictStart := strings.Index(remaining, "<<<<<<< ")
		if conflictStart > 0 {
			result.WriteString(remaining[:conflictStart])
		}

		// 提取冲突区域
		afterStart := remaining[conflictStart:]
		separatorIdx := strings.Index(afterStart, "=======")
		endIdx := strings.Index(afterStart, ">>>>>>> ")

		if separatorIdx < 0 || endIdx < 0 {
			// 格式不正确，保留原文
			result.WriteString(remaining)
			break
		}

		// 找到 <<<<<<< 行的结尾（跳过分支名和换行）
		ourStart := strings.Index(afterStart, "\n")
		if ourStart < 0 || ourStart > separatorIdx {
			ourStart = len("<<<<<<< ")
		} else {
			ourStart++ // 跳过换行符
		}

		// 提取我们的修改（到 separator 行之前）
		ourPart := afterStart[ourStart:separatorIdx]

		// 找到 separator 行的结尾
		theirStart := separatorIdx + len("=======")
		newlineAfterSep := strings.Index(afterStart[theirStart:], "\n")
		if newlineAfterSep >= 0 && theirStart+newlineAfterSep < endIdx {
			theirStart += newlineAfterSep + 1 // 跳过换行符
		}

		// 提取对方的修改（到 end marker 行之前）
		theirPart := afterStart[theirStart:endIdx]

		// 去掉对方修改末尾可能的换行（属于冲突标记的一部分，不属于实际内容）
		theirPart = strings.TrimSuffix(theirPart, "\n")

		// 根据策略选择
		var chosen string
		switch strategy {
		case "ours":
			chosen = ourPart
		case "theirs":
			chosen = theirPart
			if !strings.HasSuffix(chosen, "\n") && strings.HasSuffix(ourPart, "\n") {
				chosen += "\n"
			}
		default:
			chosen = ourPart
		}

		result.WriteString(chosen)

		// 找到 >>>>>>> 行的结尾
		endMarkerEnd := endIdx + len(">>>>>>> ")
		newlineAfterEnd := strings.Index(afterStart[endMarkerEnd:], "\n")
		if newlineAfterEnd >= 0 {
			endMarkerEnd += newlineAfterEnd + 1
		}

		// 移动到冲突区域之后
		remaining = afterStart[endMarkerEnd:]
	}

	if remaining != "" {
		result.WriteString(remaining)
	}

	return result.String()
}

// Description 返回冲突的友好描述
func (c FileConflict) Description() string {
	switch c.ConflictType {
	case ConflictEditEdit:
		return fmt.Sprintf("📄 %s：您和同事都修改了同一位置", c.FilePath)
	case ConflictEditDelete:
		return fmt.Sprintf("📄 %s：您修改了文件，但同事删除了它", c.FilePath)
	case ConflictAddAdd:
		return fmt.Sprintf("📄 %s：您和同事都新增了同名文件", c.FilePath)
	default:
		return fmt.Sprintf("📄 %s：存在冲突", c.FilePath)
	}
}

// Suggestions 返回解决建议
func (c FileConflict) Suggestions() []string {
	suggestions := []string{
		fmt.Sprintf("保留我的修改（%s 使用 ours）", c.FilePath),
		fmt.Sprintf("采用对方修改（%s 使用 theirs）", c.FilePath),
	}
	if c.AutoResolvable {
		suggestions = append(suggestions, fmt.Sprintf("自动合并（%s 使用 merge）", c.FilePath))
	}
	return suggestions
}
