ALTER TABLE backtest_runs ADD COLUMN user_id UUID REFERENCES users(id);
CREATE INDEX idx_backtest_runs_user_id ON backtest_runs(user_id);
