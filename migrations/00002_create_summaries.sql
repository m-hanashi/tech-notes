-- AI の要約を保存するテーブル（仕様書 4.4）。メモ1件につき1件。
-- 2回目以降の要約の依頼では、AI を呼ばずにここから返す。
-- 要約はメモに付属するので、メモを消すと要約も一緒に消える（ON DELETE CASCADE）。

-- +goose Up
CREATE TABLE summaries (
    note_id       uuid        PRIMARY KEY REFERENCES notes (id) ON DELETE CASCADE,
    summary       text        NOT NULL CHECK (char_length(summary) >= 1),
    model         text        NOT NULL,
    input_tokens  integer     NOT NULL CHECK (input_tokens >= 0),
    output_tokens integer     NOT NULL CHECK (output_tokens >= 0),
    latency_ms    integer     NOT NULL CHECK (latency_ms >= 0),
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE summaries;
