# GitHub のルールセットとマージの設定

段階0の手順4で設定した、main を守るルールセット（`protect-main`）と、マージの設定の解説です。用語は `GitHubの作業管理.md` も参照してください。

---

## 1. 全体像：main に入る道を1本にする

```
作業用ブランチ ──push──▶ GitHub のブランチ ──PR──▶ （Squash）──▶ main
                                                 ▲
main に直接 push ──────────────────────────────── ✕ ルールセットが拒否
```

ただし、例外が1つあります。開いている PR の先頭のコミットを main に push すると、PR のマージとして受け入れられる場合があります（2節の「PR の先頭のコミットの例外」）。

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
| Require a pull request | 有効 | main への変更は、PR のマージだけ | 直接の push を拒否（例外は下の小見出し） |
| └ Required approvals | 0 | 他人の承認は求めない | 1人では自分の PR を承認できないため。代わりに、PR のひな形の確認の欄を関所にする |
| └ Allowed merge methods | Squash だけ | PR の取り込み方を1つに固定 | 3節を参照 |

### 0のままにした細かい項目

| 項目 | 意味 | 0（無効）にした理由 |
|---|---|---|
| Dismiss stale approvals | 承認のあとに push があったら、承認を取り消す | 承認そのものを使わない |
| Require approval of the most recent push | 最後に push した人以外の承認を求める | 1人では満たせない |
| Require conversation resolution | PR のコメントのやりとりを、すべて「解決済み」にするまでマージできない | 1人では使う場面が少ない。必要になったら足す |
| Require code owner review | ファイルごとの担当者の承認を求める | 担当者が1人 |

### 設定の画面にはないが、API で見えた項目

`require_extra_approval_for_unattributed_changes: true` という項目が、初期設定で有効になっていました。名前から、「人に結びつかない変更（AI が作った変更など）に、追加の承認を求める」ものと思われます。ただし、GitHub の公式の説明を確認できていません。PR #12 と PR #14 では、追加の承認を求められずにマージできました。必要な承認の数が0のため、今の設定では影響しないと考えられます。

### PR の先頭のコミットの例外（2026-11-15 に確認）

「Require a pull request」のルールには、次の例外があります。main に push したコミットが、開いている PR の先頭のコミットと同じで、その PR がルールの条件をすべて満たしているとき、GitHub はこの push を「その PR を手元でマージした」とみなして受け入れます。GitHub の画面を使わずに、コマンドで PR をマージするための仕組みです。

PR #12 で、直接の push が拒否されることを確かめようとして、この例外の条件を満たしてしまいました。そこで、PR に含まれていないコミットでも試しました。

| 試したこと | 結果 |
|---|---|
| `git push origin 1-add-permissions:main`。送ったのは PR #12 の先頭のコミット | 受け入れられた。PR #12 はマージ済みになり、Issue #1 も閉じた。Squash ではなく、作業用ブランチのコミットがそのまま main に入った |
| 手元の main で空のコミットを作り、`git push`。どの PR にも含まれていないコミット | 拒否された（`GH013: Repository rule violations found` と `Changes must be made through a pull request.`） |

今は承認の数が0で、CI の成功も条件にしていないため、開いている PR の先頭のコミットは、いつでもこの方法で main に入れられます。段階2で「Require status checks」を足すと、CI が通っていない PR のコミットは、この方法でも入れられなくなります。

ルールを試すときは、試したい条件だけが当てはまる状況を作ります。今回は「PR を通さない変更」を試したかったので、最初から PR に含まれていないコミットを使うべきでした。

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

Squash では、作業用ブランチのコミット（A・B・C）と main のコミット（●）が別物になります。そのため、手順3では毎回「まだマージされていない」（`not fully merged`）と警告が出て、ブランチは消えません。そのときは、`git diff main ブランチ名` で2つのブランチのファイルに差がないことを確かめてから、`git branch -D ブランチ名`（強制）で消します。

---

## 4. 最初の PR で起きたこと

| PR | マージの仕方 | 結果 |
|---|---|---|
| #12（Issue #1） | 直接の push の試験で、PR の先頭のコミットを main に送った | 2節の例外により、Squash ではない形でマージされた。main のコミットの本文は、PR の説明ではなくコミットメッセージになった |
| #14（Issue #2） | 画面で「Squash and merge」を押した | 予定どおり、PR の題名（`(#14)` 付き）が main のコミットの題名に、PR の説明が本文になった。`Co-authored-by` の行も引き継がれた |

どちらも、Issue は `Closes #番号` で自動で閉じ、GitHub 上の作業用ブランチは自動で削除されました。

Squash のとき、GitHub はコミットの本文の長い行を折り返します。そのため、main のコミットの本文では、PR の説明の途中に改行が入ることがあります。
