# Wallet MVP Design

## 1. Project Goal
这个项目不是 full production wallet。
第一版目标是做一个：

```text
single-chain Ethereum native ETH deposit scanner
+
balance ledger MVP
```
也就是：

```text
单链 Ethereum native ETH 充值监听
+
幂等入账
+
余额流水
```
核心目标不是“做完整钱包”，而是理解 wallet backend 里最重要的 fund-safety boundary。

重点能力：

```text
deposit detection 充值监听
confirmation-based crediting 基于确认数的入账
idempotent balance update 幂等余额更新
balance ledger 余额账本
wallet scanner cursor 钱包扫描游标
```
---
## 2. MVP Scope
This MVP includes:
```text
Native ETH deposit detection
User deposit address mapping
Deposit status tracking
Confirmation-based crediting
Idempotent deposit record creation 幂等生成充值记录
Balance account update 
Balance ledger recording
Wallet scanner cursor
```
---
## 3. Out of Scope
This MVP does not include:
```text
ERC20 deposits
Withdrawals
Hot wallet / cold wallet management
Private key management
Transaction signing 交易签名
Gas management
Multi-chain support
Full reorg rollback automation 自动化完整重组回滚
Risk control system
Frontend UI
```
---
## 4. Core Data Flow
```text
block-explorer-backend
  -> completed block + successful native incoming transaction 成功的原生转入交易
  -> wallet scanner
  -> match deposit address
  -> create or update deposit record
  -> wait for confirmation depth
  -> credit deposit idempotently 幂等入账充值
  -> update balance account
  -> write balance ledger
  -> move wallet scan cursor
```
A transaction can only be credited after it passes:

```text
receipt_status = 1
block completed
to_address belongs to a user deposit address
confirmation depth is satisfied
deposit record is unique
balance ledger is written successfully
```
---
## 5. Database Tables
### users
```text
purpose:
- Represents an internal platform user.
- MVP does not implement full authentication or registration.

fields:
- id
- status
- created_at
- updated_at

primary key:
- id

unique constraints:
- none

indexes:
- index(status)

business rules:
- user is the owner of deposit_addresses and balance_accounts.
- only active users should be assigned new deposit addresses.
```
### deposit_addresses
```text
purpose:
- Maps a blockchain deposit address to an internal user.
- Scanner uses this table to decide whether tx.to belongs to the platform.

fields:
- id
- user_id
- chain_id
- address
- address_lower
- status
- created_at
- updated_at

primary key:
- id

foreign keys:
- user_id -> users.id

unique constraints:
- unique(chain_id, address_lower)

indexes:
- index(user_id, chain_id)
- index(status)

business rules:
- address stores the original/checksum address for display.
- address_lower must be lower(address).
- scanner matches tx.to by chain_id + address_lower.
- one address must never belong to multiple users on the same chain.
- disabled addresses should not be reused or reassigned to another user.
```
### deposits
Stores detected native ETH deposits.
```text
purpose:
- Records an on-chain native ETH deposit detected by the scanner.
- A deposit is a chain event first, not a balance change.

fields:
- id
- user_id
- chain_id
- deposit_address_id
- tx_hash
- block_number
- block_hash
- from_address
- to_address
- amount_wei
- status
- receipt_status
- credited_at
- created_at
- updated_at

primary key:
- id

foreign keys:
- user_id -> users.id
- deposit_address_id -> deposit_addresses.id

unique constraints:
- unique(chain_id, tx_hash)

indexes:
- index(user_id, created_at)
- index(chain_id, status, block_number)
- index(chain_id, block_number)
- index(deposit_address_id)

business rules:
- native ETH deposit is uniquely identified by chain_id + tx_hash.
- only receipt_status = 1 transactions can become confirming deposits.
- amount_wei must be stored as integer wei, not float.
- status starts as confirming after scanner detects a valid deposit.
- credited means balance_ledgers has been inserted and balance_accounts has been updated atomically.
- credited_at is only set when status becomes credited.
- confirmation count is computed from latest scanned block and deposit.block_number, not stored as a fixed field.
```
### balance_accounts
```text
purpose:
- Stores the current internal balance of a user for one chain and one asset.
- This is the user's platform-side balance, not directly the chain balance.

fields:
- id
- user_id
- chain_id
- asset_symbol
- available_balance
- frozen_balance
- created_at
- updated_at

primary key:
- id

foreign keys:
- user_id -> users.id

unique constraints:
- unique(user_id, chain_id, asset_symbol)

indexes:
- index(user_id, chain_id)

business rules:
- one user can only have one balance account for the same chain_id and asset_symbol.
- MVP only supports asset_symbol = ETH.
- available_balance and frozen_balance must be stored as integer wei.
- available_balance >= 0.
- frozen_balance >= 0.
- balance_accounts must only be updated together with balance_ledgers in the same database transaction.
- deposit credit only increases available_balance.
```
### balance_ledgers
```text
purpose:
- Records every internal balance change.
- Explains why the user's balance changed.
- Provides auditability and idempotency protection for crediting deposits.

fields:
- id
- user_id
- chain_id
- asset_symbol
- amount_wei
- direction
- reason
- source_type
- source_id
- created_at

primary key:
- id

foreign keys:
- user_id -> users.id

unique constraints:
- unique(source_type, source_id)

indexes:
- index(user_id, created_at)
- index(chain_id, asset_symbol)
- index(source_type, source_id)

business rules:
- amount_wei must be positive integer wei.
- direction can be credit or debit.
- deposit credit uses direction = credit.
- deposit credit uses reason = deposit_credit.
- deposit credit uses source_type = deposit.
- deposit credit uses source_id = deposits.id.
- one deposit can create at most one balance ledger.
- balance_ledgers must be inserted in the same database transaction as balance_accounts update.
```
### wallet_scanner_cursors
```text
purpose:
- Records scanner progress.
- Allows scanner to restart safely without missing blocks.
- Helps detect reorg or chain discontinuity.

fields:
- id
- chain_id
- scanner_name
- last_scanned_block_number
- last_scanned_block_hash
- created_at
- updated_at

primary key:
- id

unique constraints:
- unique(chain_id, scanner_name)

indexes:
- index(chain_id)

business rules:
- last_scanned_block_number means the latest block fully processed and committed.
- scanner_name distinguishes different scanner tasks.
- last_scanned_block_hash is used to detect reorg or chain discontinuity.
- scanner should update cursor only after the whole block is processed successfully.
- scanner restarts from last_scanned_block_number + 1 after verifying last_scanned_block_hash still matches the chain.
- if hash mismatch is detected, scanner should stop and report reorg risk.
- MVP does not automatically rollback reorged deposits.
```
---
## 6. Deposit Status Machine
```text
  -> confirming
  -> credited
  -> reorged
failed
```
Status meaning:
### detected
```text
The scanner found a possible deposit transaction.scanner 
发现疑似充值。但还没有完成所有检查。
```
### confirming
```text
The transaction is successful and locally complete,
but still waiting for enough confirmations.
receipt_status = 1
block completed
to_address matched
但是 confirmation depth 还不够。
不能进入 available_balance。
```
### credited
```text
The deposit has been credited to the user's balance
through an idempotent ledger operation.
已经通过幂等入账流程。
已经更新 balance_accounts。
已经写入 balance_ledger。
```
### failed
```text
- Internal crediting failed.
- It should be retryable or manually reviewed.
- receipt_status = 0 transactions should not become valid deposits in MVP.
receipt_status = 0
amount below minimum
address not matched
chain_id mismatch
```
### reorged
```text
The deposit was affected by a chain reorg
and should no longer be treated as valid without rechecking.
这笔充值受到 reorg 影响。
不能继续当作正常 credited deposit。
需要 recheck / reversal / correction。
```
---
## 7. Idempotency Design
For native ETH deposits, the idempotency key is:
```text
chain_id + tx_hash
同一条链上的同一个 tx_hash 只能入账一次。
```
This prevents the same native ETH deposit transaction from being credited more than once.
The crediting operation must be atomic:
```text
Scan block transaction:
- insert confirming deposit idempotently
- update wallet_scanner_cursor after the whole block is processed

Credit deposit transaction:
- lock confirming deposit
- insert balance_ledger
- update balance_account
- mark deposit as credited
```
正确做法：
```text
BEGIN
check deposit idempotency
update balance_accounts
insert balance_ledger
mark deposit as credited
COMMIT
```
如果中间失败：
```text
ROLLBACK
```
For future ERC20 deposits, the idempotency key should be:
```text
chain_id + token_contract_address + tx_hash + log_index
because one transaction can emit multiple ERC20 Transfer events.
ERC20 不能只用 tx_hash。
因为一笔 transaction 里可能有多个 Transfer event。
所以要加 log_index。
```
---
## 8. Current Limitations
```text
Native ETH deposits only
Single-chain Ethereum only
No ERC20 Transfer event indexing yet
No withdrawal flow
No private key or signing logic
No hot wallet / cold wallet management
No full automatic reorg rollback yet
No frontend UI
No production-grade risk control or alerting
第一版只是充值监听 + ledger MVP。
不是生产级钱包。
```
最重要的 production gaps:
```text
confirmation policy
reorg rollback
ledger reversal
withdrawal state machine
risk control
observability
manual repair tools
```
---
## 9. Roadmap
Planned improvements:
```text
Add ERC20 Transfer event indexing
Add withdrawal request and withdrawal state machine
Add hot wallet / cold wallet architecture
Add reorg rollback and balance correction workflow
Add risk control and audit logs
Add multi-chain support
Add admin tools for deposit repair and manual review
先做 ERC20 deposit。
再做 withdrawal。
再做 hot wallet / cold wallet。
再做 reorg rollback 和 risk control。
最后考虑 multi-chain 和 admin repair tools。
```
# Core Principle
The most important goal is not:
```text
find a transaction
The real goal is:
safely, uniquely, and audibly change user balance
钱包系统最危险的动作不是“查到交易”，
而是“修改用户余额”。
```
所以不要这样设计：
```text
found tx
  -> update balance directly
```
应该这样设计：
```text
found tx
  -> create deposit record
  -> wait for confirmation depth
  -> idempotency check
  -> database transaction:
       update balance account
       insert balance ledger
       mark deposit credited
```
---
# Key Sentences To Remember
```text
receipt_status = 1 only means on-chain execution success.
```
```text
completed block means local index completeness, not chain finality.
```
```text
confirmation depth or finalized-block policy protects against reorg risk.
```
```text
deposit record stores deposit facts.
```
```text
balance_accounts stores current balance.
```
```text
balance_ledger stores why balance changed.
```
```text
crediting must be idempotent and atomic.
```
```text
Native ETH idempotency key: chain_id + tx_hash.
```
```text
ERC20 idempotency key: chain_id + token_contract_address + tx_hash + log_index.
```
```text
1. A deposit must come from an on-chain transaction.
2. A native ETH deposit is uniquely identified by chain_id + tx_hash.
3. A deposit can only be credited once.
4. Balance changes must be recorded through balance_ledgers.
5. balance_accounts cannot be updated without a ledger.
6. Credit deposit must be executed in one database transaction.
7. Scanner only discovers deposits; credit service updates ledger and balance.
8. Reorg rollback is out of MVP scope. MVP only detects and stops/marks.
```