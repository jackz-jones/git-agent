// Package internal 包含内部逻辑定义
package internal

import "fmt"

var (
	// CommitID git commit hash
	CommitID = ""

	// BuildTime when to build
	BuildTime = ""

	// Version what version
	Version = ""

	// http://patorjk.com/software/taag/#p=display&f=Standard&t=GIT%20AGENT&x=none
	logoStr = `
  ____ ___ _____      _    ____ _____ _   _ _____ 
 / ___|_ _|_   _|    / \  / ___| ____| \ | |_   _|
| |  _ | |  | |     / _ \| |  _|  _| |  \| | | |  
| |_| || |  | |    / ___ \ |_| | |___| |\  | | |  
 \____|___| |_|   /_/   \_\____|_____|_| \_| |_|  
                                                  `
)

// VersionInfo print current cmd version
func VersionInfo() string {
	return fmt.Sprintf("\n%s\n\nCurrent version: %s\nCommit hash: %s\nBuild time: %s\n\n", logoStr,
		Version, CommitID, BuildTime)
}

// ShortCommitID 返回短格式的 commit hash（前7位）
func ShortCommitID() string {
	if len(CommitID) > 7 {
		return CommitID[:7]
	}
	return CommitID
}
