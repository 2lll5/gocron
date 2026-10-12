# Agent Instructions

## Google Drive Build 交付規範（所有 Agent 必須遵守）
- Google Drive Build 唯一位置為 `AI Project/gocron/`，資料夾名必須與 GitHub Repo 同名（AI Project ID `1Yhfro6Ss5TSCXCVIZIHEu5h7V1gL8b27`）。不要把不同專案或版本散放在 AI Project 頂層。
- **最新完整且經可執行測試的 Build 直接放專案根目錄**，包含 ZIP／EXE、SHA-256、全數分卷、必要還原腳本及說明；不同平台的當前版本可並存。驗證上傳後檔案名稱、大小、父資料夾、checksum；缺 ZIP、只有 checksum 或少分卷屬不完整，不得取代上一個完整版本。
- **重要版本差異**（功能、架構、重大美術或相容性）才建立 `vX.Y.Z-說明/` 子目錄；普通舊版本一律移入 `歷史封存/`，不刪檔，也不每次 Build 都建立新版本資料夾。
- 所有開發推送 GitHub；昂貴 Build/發布 Actions 優先 `workflow_dispatch` 或 `release: published`，不要每次 push main 都執行；如未 Build／測試／上傳成功，不可宣稱完成。回報 Drive 資料夾連結、版本、檔案、校驗狀態。
**English:** Codex must keep latest complete verified build(s) at `AI Project/gocron/`, archive routine old builds in `歷史封存/`, keep only major milestone version folders. Never promote incomplete archives.
