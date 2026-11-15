# GitHub のルールセットとマージの設定

段階0の手順4で設定した、main を守るルールセット（`protect-main`）と、マージの設定の解説です。用語は `GitHubの作業管理.md` も参照してください。

---

## 1. 全体像：main に入る道を1本にする

```
作業用ブランチ ──push──▶ GitHub のブランチ ──PR──▶ （Squash）──▶ main
                                                 ▲
main に直接 push ──────────────────────────────── ✕ ルールセットが拒否
```

| 守り | 誰を止めるか | どこで止めるか |
|---|---|---|
| Claude Code の権限設定（手順3） | AI | 手元の PC（実行の直前） |
| ルールセット（手順4） | **全員**（利用者自身も含む） | GitHub（受け取るとき） |

権限設定は「AI に鍵を渡さない」、ルールセットは「金庫そのものの錠」です。

---

## 2. ルールセット `protect-main` の中身

| 設定 | 値 | 意味 | 起きる場面 |
|---|---|---|---|
| Enforcement | Active | ルールを実際に適用する | Evaluate にすると「破られたら記録するだけ」になる |
| Bypass list | なし | 管理者も例外にしない | 利用者自身が `git push origin main` を打っても拒否される |
| Target | Default branch（main） | 対象は main だけ | 作業用のブランチには、何も制限がかからない |
| Restrict deletions | 有効 | main を消せない | `git push origin --delete main` などを拒否 |
| Block force pushes | 有効 | main の履歴を上書きできない | `git push --force` で main の過去を書き換えることを拒否 |
| Require a pull request | 有効 | main への変更は、PR のマージだけ | 直接の push を拒否 |
| └ Required approvals | 0 | 他人の承認は求めない | 1人では自分の PR を承認できないため。代わりに、PR のひな形の「自分の言葉で書く欄」を関所にする |
| └ Allowed merge methods | Squash だけ | PR の取り込み方を1つに固定 | 3節を参照 |

### 0のままにした細かい項目

| 項目 | 意味 | 0（無効）にした理由 |
|---|---|---|
| Dismiss stale approvals | 承認のあとに push があったら、承認を取り消す | 承認そのものを使わない |
| Require approval of the most recent push | 最後に push した人以外の承認を求める | 1人では満たせない |
| Require conversation resolution | PR のコメントのやりとりを、すべて「解決済み」にするまでマージできない | 1人では使う場面が少ない。必要になったら足す |
| Require code owner review | ファイルごとの担当者の承認を求める | 担当者が1人 |

### 設定の画面にはないが、API で見えた項目

`require_extra_approval_for_unattributed_changes: true` という項目が、初期設定で有効になっていました。名前から、「人に結びつかない変更（AI が作った変更など）に、追加の承認を求める」ものと思われます。ただし、GitHub の公式の説明を確認できていません。必要な承認の数が0のため、今の設定では影響しないと考えられます。最初の PR で、実際の動きを確かめます。

### あとで足すもの

| 時期 | 足すルール | 理由 |
|---|---|---|
| 段階2 | Require status checks（CI の成功を必須にする） | CI ができるまでは、確かめる対象がない |

---

## 3. マージの方法：Squash

### 3つの方法の違い

作業用ブランチで3回コミットした PR を取り込む場合：

```
作業用ブランチ:  A ─ B ─ C        （A：作る、B：直す、C：typo を直す）

Squash:        main ─ ● ← A・B・C をまとめた1つのコミット
Merge commit:  main ─────── M ← 「マージした」コミット
                  └ A ─ B ─ C ┘   （A・B・C もすべて残る）
Rebase:        main ─ A' ─ B' ─ C'（A・B・C を main の先に並べ直す）
```

### Squash を選んだ理由

- 「1 Issue = 1 PR = main の1コミット」になり、対応が単純になる
- 途中のコミット（A・B・C）は PR の画面に残るので、経過も失われない
- 切り戻し（第7章）を、コミット単位で考えやすい

### リポジトリ全体の設定（Settings → General）

| 設定 | 値 | 意味 |
|---|---|---|
| Allow merge commits | 無効 | |
| Allow squash merging | 有効 | |
| └ コミットのタイトル | PR のタイトル | 例：`権限設定を追加する (#3)` |
| └ コミットの本文 | PR の本文 | `Closes #番号`、AI に任せた部分などが、main のコミットに残る |
| Allow rebase merging | 無効 | |
| Automatically delete head branches | 有効 | マージ後、GitHub 上の作業用ブランチを自動で消す |

ルールセットの「Squash だけ」と、全体の設定の「Squash だけ」は、同じことを2か所で決めています。全体の設定は、**マージのボタンに何を表示するか**を決めます。ルールセットは、**main に対して何を許すか**を決めます。

### 手元のブランチの片付け

GitHub 上のブランチは自動で消えますが、**手元の PC のブランチは残ります**。マージのあとに、次の操作で片付けます（最初の PR のときに、1つずつ説明します）。

1. `git switch main`：main に移る
2. `git pull`：GitHub の main（Squash されたコミット）を取り込む
3. `git branch -d ブランチ名`：手元の作業用ブランチを消す

Squash では、作業用ブランチのコミット（A・B・C）と main のコミット（●）が別物になります。そのため、手順3で「まだマージされていない」と警告が出ることがあります。そのときは `-D`（強制）で消してよいことを、その場で確かめてから使います。

---

## 4. 最初の PR で起きること（予定）

| 順番 | 誰が | 操作 | ルールの働き |
|---|---|---|---|
| 1 | AI | Issue を作る | |
| 2 | AI | ブランチを作り、変更をコミットする | |
| 3 | 利用者 | `git push -u origin ブランチ名` | 作業用ブランチなので通る |
| 4 | AI | PR の下書きを作る（`gh pr create`） | |
| 5 | 利用者 | 確認の欄を自分の言葉で書く | |
| 6 | 利用者 | GitHub の画面で「Squash and merge」を押す | 「Squash and merge」のボタンだけが表示される |
| 7 | — | Issue が自動で閉じ、作業用ブランチが消える | |

あわせて、手元の main から直接 push すると拒否されることを、1度だけ試して確かめます（試すのは利用者です）。
