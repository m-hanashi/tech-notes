-- メモを保存するテーブル（仕様書 4.4）。
-- 文字数の決まりは、アプリでも確かめる（#26）。DB の CHECK 制約は、最後の守り。

-- +goose Up
CREATE TABLE notes (
    id         uuid        PRIMARY KEY,
    title      text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body       text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 10000),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE notes;
