CREATE TABLE equipment_model (
  id            UUID PRIMARY KEY,
  name          TEXT NOT NULL,
  category      TEXT NOT NULL,
  max_loan_days INT  NOT NULL CHECK (max_loan_days > 0)
);

CREATE TABLE item (
  id               UUID PRIMARY KEY,
  model_id         UUID NOT NULL REFERENCES equipment_model(id),
  inventory_number TEXT NOT NULL UNIQUE,
  status           TEXT NOT NULL CHECK (status IN ('available','borrowed','maintenance','lost'))
);

CREATE TABLE loan (
  id              UUID PRIMARY KEY,
  item_id         UUID NOT NULL REFERENCES item(id),
  borrower_sub    TEXT NOT NULL CHECK (length(borrower_sub) <= 128),
  borrowed_at     TIMESTAMPTZ NOT NULL,
  due_at          TIMESTAMPTZ NOT NULL,
  returned_at     TIMESTAMPTZ,
  return_state    TEXT CHECK (return_state IN ('ok','damaged')),
  extensions_used INT NOT NULL DEFAULT 0 CHECK (extensions_used >= 0)
);

CREATE UNIQUE INDEX one_active_loan_per_item ON loan(item_id) WHERE returned_at IS NULL;
CREATE INDEX loan_borrower_idx ON loan(borrower_sub);
