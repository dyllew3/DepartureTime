CREATE TABLE TerminalRecords (
  terminal STRING NOT NULL,
  wait_len INT NOT NULL,
  timestamp TIMESTAMPTZ NOT NULL,
  CONSTRAINT "primary" PRIMARY KEY (terminal, timestamp)
);