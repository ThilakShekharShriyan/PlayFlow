-- Create accounts table
CREATE TABLE accounts (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL CHECK (type IN ('ASSET', 'LIABILITY', 'REVENUE', 'EXPENSE')),
    currency VARCHAR(3) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create transactions table
CREATE TABLE transactions (
    id VARCHAR(255) PRIMARY KEY,
    description TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create ledger_entries table (append-only)
CREATE TABLE ledger_entries (
    id VARCHAR(255) PRIMARY KEY,
    transaction_id VARCHAR(255) NOT NULL REFERENCES transactions(id),
    entry_index INT NOT NULL,
    account_id VARCHAR(255) NOT NULL REFERENCES accounts(id),
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (transaction_id, entry_index)
);

-- Indexes
CREATE INDEX idx_ledger_entries_transaction_id ON ledger_entries(transaction_id);
CREATE INDEX idx_ledger_entries_account_id ON ledger_entries(account_id);
CREATE INDEX idx_ledger_entries_created_at ON ledger_entries(created_at);
CREATE INDEX idx_transactions_created_at ON transactions(created_at);

-- Insert default system accounts
INSERT INTO accounts (id, name, type, currency) VALUES
    ('acc_customer_cash', 'Customer Cash', 'ASSET', 'USD'),
    ('acc_merchant_receivable', 'Merchant Receivable', 'ASSET', 'USD'),
    ('acc_platform_fee', 'Platform Fee', 'REVENUE', 'USD'),
    ('acc_merchant_payable', 'Merchant Payable', 'LIABILITY', 'USD');
