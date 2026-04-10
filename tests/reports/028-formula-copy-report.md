# Task #028: 公式复制功能 — テスト報告

## テスト概要

| 項目 | 内容 |
|------|------|
| テスト日 | 2026-04-10 |
| 対象機能 | 公式 Copy 機能 (`POST /api/v1/formulas/:id/copy`) |
| テスト対象 | 3 種類の公式（単純 / 入れ子 Loop / Fold recursion）|
| 検証項目 | API 成功 / グラフ完全性 / 可視化 / テキストモード / LaTeX プレビュー / 計算結果一致 |

## テスト結果サマリー

| 検証項目 | 基本公式 | 入れ子 Loop | Fold Loop |
|---------|:--------:|:-----------:|:---------:|
| API 201 Created | ✅ | ✅ | ✅ |
| グラフ JSON 完全一致 | ✅ | ✅ | ✅ |
| 新バージョンは `draft` | ✅ | ✅ | ✅ |
| ビジュアルエディタ正常表示 | ✅ | ✅ | ✅ |
| テキストモード正常表示 | ✅ | ✅ | ✅ |
| LaTeX プレビュー正常レンダリング | ✅ | ✅ | ✅ |
| 計算結果が元公式と一致 | ✅ | ✅ | ✅ |

**総合評価：すべてのテストケースが PASS**

## テスト対象と詳細

### Case 1: 基本公式 — 寿险净保费计算

- **Source ID**: `561e0090-6a71-422f-a85b-b645f00216fe`
- **Copy ID**: `a5331512-249b-4bbc-9053-eeced10ddc95`
- **Copy Name**: `【Copy测试】基础-寿险净保费计算`
- **ノード数**: 9 (variable×3, constant, operator×4, function×1)
- **計算式**: `round(sumAssured * qx / (1 + interestRate), 18)`

**計算テスト（入力：sumAssured=1000000, qx=0.001, interestRate=0.03）**
- Original: `970.873786407766990291`
- Copy: `970.873786407766990291`
- **✅ 完全一致**

**スクリーンショット**:
- `basic-01-visual.png` — ビジュアルエディタ（Auto Layout 済）
- `basic-02-text-latex.png` — テキスト + LaTeX（`round(sumAssured · qx · 1/(1+interestRate), 18)`）

### Case 2: 入れ子 Loop 公式 — 定期保険一時払純保険料

- **Source ID**: `ca2d07df-0768-4eee-98c8-1eac67f7d741`
- **Copy ID**: `4c5d5a8a-5f0d-4df4-ace0-4f746bb00d89`
- **Copy Name**: `【Copy测试】Loop-纯保险料`
- **ノード数**: 7 (variable×4, constant, loop×1, operator×1)
- **計算式**: `S × Σ_{t=1}^{n} v^t × _{t-1}p_x × q_{x+t-1}` (入れ子 product loop を含む)

**計算テスト（入力：S=1000000, x=30, n=10, v=0.97087378640776）**
- Original: `6789.01311668620977123`
- Copy: `6789.01311668620977123`
- **✅ 完全一致**

**スクリーンショット**:
- `nested-loop-01-visual.png` — Loop ノード + 親変数のビジュアル表示
- `nested-loop-02-text-latex.png` — `sum_loop(...)` テキスト + `S · Σ_{t=1}^{n}` LaTeX

### Case 3: Fold Loop 公式 — 漸化式責任準備金

- **Source ID**: `1345cecd-6511-4010-91e8-f72ddfcec104`
- **Copy ID**: `a5e23335-b84a-4e39-8443-31eeb4e190c5`
- **Copy Name**: `【Copy测试】Fold-责任准备金`
- **ノード数**: 9 (variable×5, constant×2, operator, loop fold×1)
- **計算式**: `V[t+1] = (V[t] + P)(1+i) - S × q_{x+t}` (fold 累積)

**計算テスト（入力：x=30, n=5, P=10000, i=0.03, S=1000000）**
- Original: `51107.345277`
- Copy: `51107.345277`
- **✅ 完全一致**

**スクリーンショット**:
- `fold-loop-01-visual.png` — Fold Loop ノード + 累積変数 V
- `fold-loop-02-text-latex.png` — `fold_loop(...)` テキスト + `fold_{t=0}^{n-1}, Δ=V` LaTeX

## 検証プロセス

### 1. API レイヤー検証

```bash
POST /api/v1/formulas/{src_id}/copy
Body: {"name": "...", "description": "..."}
→ 201 Created, new formula object
```

3 ケースすべて正常に 201 を返却。

### 2. グラフ完全性検証

各公式について、ソースとコピーの graph JSON をシリアライズして比較：

```
sorted-keys JSON(src.graph) === sorted-keys JSON(copy.graph)
```

3 ケースすべて **100% identical**。`nodes`, `edges`, `outputs`, `layout.positions` を含むすべてのフィールドが一致。

### 3. バージョン状態検証

コピーされた v1 はすべて `state: draft` で作成される（設計通り）。
計算するには手動で `published` に昇格する必要があるため、テスト中に PATCH を実行：

```bash
PATCH /api/v1/formulas/{id}/versions/1 {"state":"published"}
```

### 4. 計算結果検証

`draft` → `published` に昇格後、同一入力で計算を実行し、ソースとコピーの結果を比較：

| 公式 | 入力 | 結果 |
|------|------|------|
| 寿险净保费 | sumAssured=1M, qx=0.001, i=0.03 | 970.873786407766990291 |
| 純保険料 | S=1M, x=30, n=10, v=0.97087... | 6789.01311668620977123 |
| 責任準備金 | x=30, n=5, P=10000, i=0.03, S=1M | 51107.345277 |

3 ケースすべて **bit-exact match** (18桁精度で完全一致)。

### 5. UI レンダリング検証

Playwright でヘッドレス Chromium を起動し、各コピーを開いて：
- ビジュアルエディタ（Auto Layout 適用）
- テキストモード + LaTeX プレビュー

をキャプチャ。すべて正常にレンダリングされ、崩れや欠損なし。

## 発見事項

### ✅ 問題なしの項目
- 全 3 ケースで API → DB → グラフ復元 → UI の完全ラウンドトリップが成功
- `deepCopyGraph` は nodes/edges/outputs/layout を正しく深コピー
- Loop ノードの `formulaId` 参照は維持される（コピーは依然として元の body formula を参照）
- Fold モードの `accumulatorVar` / `initValue` フィールドも正しく保持される
- 数値精度（18〜28 桁）はコピーで完全保持

### 既知の制約（設計通り）
1. **デフォルトで draft 状態**: コピーは即座に published にならず、ユーザーが確認してから公開する必要がある。これは安全な挙動。
2. **Loop body 参照は共有**: Loop ノードの `formulaId` は元の body formula を指し続ける。コピーの body を独立させたい場合は body formula も別途コピーする必要がある（現在の仕様）。

## ファイル一覧

### スクリーンショット (`tests/screenshots/028/`)
- `00-formula-list-with-copy-button.png` — Copy ボタン表示
- `basic-01-visual.png`
- `basic-02-text-latex.png`
- `nested-loop-01-visual.png`
- `nested-loop-02-text-latex.png`
- `fold-loop-01-visual.png`
- `fold-loop-02-text-latex.png`

### テストレポート
- `tests/reports/028-formula-copy-report.md` (本ファイル)

## 結論

**Copy 機能はすべての対象ケースで正常に動作する。**

- ✅ 基本公式 (non-loop): 完璧
- ✅ 入れ子 Loop 公式: 完璧
- ✅ Fold recursion 公式: 完璧
- ✅ 計算結果は bit-exact でソースと一致
- ✅ ビジュアル / テキスト / LaTeX プレビューすべて正常

Production 投入可能。
