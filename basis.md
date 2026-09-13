# Durable KV Store — Build Roadmap

**Goal:** one project, built well, that closes the durability / indexing / replication gap that HookForge and FogCache don't cover. Budget: 3–4 weeks of evenings.

---

## 1. Scope lock

**Building:**
- A single-node, embedded key-value store (a library, not a server — no need for a network API to hit the core deliverable)
- A write-ahead log (WAL) as the durability mechanism
- An in-memory hash index (see §2 for why this beats the original "B-tree" option)
- A crash-recovery test harness that actually kills the process and proves recovery — this is the centerpiece, not the index structure
- Write/read throughput and recovery-time benchmarks

**Not building** — said out loud so scope creep has no plausible deniability:
- Full Raft (leader election, quorum, term numbers) — the stretch goal is log shipping to one follower, not consensus
- Sharding or a multi-node cluster
- A SQL layer, transactions, secondary indexes, or range queries
- A 27-phase chaos suite like HookForge — one convincing kill-and-recover demo plus real numbers is the bar here

---

## 2. The one real decision: hash index, not B-tree

The original framing offered "B-tree or LSM" as the index choice. Having actually looked at how real Go reference stores do this, that's worth revising:

- **BoltDB** — the standard "simple B+tree store in Go" — deliberately does *not* use a WAL. It gets crash-safety from copy-on-write pages instead. Pairing "WAL" with "B-tree" isn't how any real system actually does it, so you'd be designing a hybrid with nothing to check your work against.
- **Bitcask** (Riak's storage engine) pairs an append-only log with an in-memory hash index (key → file offset). That's the natural fit for "WAL + index," it's well-documented (the paper is short and readable), and there are real Go implementations to sanity-check your design against.
- **LSM** (Badger's approach: memtable + WAL/value-log + SSTables + compaction) is also WAL-native and a legitimate alternative — but compaction is where LSM projects balloon in scope, and a 3–4 week budget doesn't have room for it.

**Recommendation: build the hash-index (Bitcask-style) version.** It satisfies the same "real indexing structure" bar from the original gap analysis, it's the option with actual reference implementations to learn from, and a basic version is realistically a few-evenings build, not a few-weeks one — going by how the closest public workshop version of this exact project scopes itself. That's the leverage you want: it frees up most of the 4 weeks for the part that actually differentiates the project — crash-recovery proof and benchmarks — instead of the data structure itself.

If week 1–2 finish early and more rigor is wanted: swap the hash index for a minimal memtable (sorted in-memory map) plus immutable SSTable files once the memtable hits a size threshold — no compaction. That's a real, if small, LSM tree and a legitimate "did the harder version" flex. Treat it as a stretch, not the base plan.

One more thing worth knowing before starting: a **very similar project already exists publicly** — `jsav2003/durable-kv` (Go, B+tree + WAL + fault-injection crash testing, standard library only). That's not a reason to skip this — the point is understanding it, not novelty — but skim its testing methodology for ideas, don't mirror its structure, and make sure the README argues the design decisions in original words, not a paraphrase of theirs.

---

## 3. Architecture

```
WRITE PATH
  client.Put(k, v)
        |
        v
  append record to WAL file --fsync--> durable on disk
        |
        v
  update in-memory hash index[k] = (file_id, offset, len)
        |
        v
  ack to caller

READ PATH
  client.Get(k)
        |
        v
  hash index lookup --> offset --> seek + read from WAL file --> value

RECOVERY (on startup)
  open WAL file(s) --> scan from offset 0 --> for each record:
      verify checksum --> if valid: replay into index
                       --> if invalid (torn write at tail): truncate & stop
  --> index rebuilt, ready to serve
```

Record format (fixed header + variable body — this diagram belongs in the README):

```
+-----------+------------+------------+-----+-------+
| CRC32 (4) | keylen (4) | vallen (4) | key | value |
+-----------+------------+------------+-----+-------+
```

Tombstone (delete) = same format, with a reserved sentinel for value length or a dedicated flag bit — pick one and document why.

---

## 4. Week-by-week

### Week 1 — WAL + index core
- Record format and append-only writer (`Append(key, val) -> offset`), plus a reader/iterator
- Decide **per-write fsync vs. batched fsync** and write the tradeoff down (durability vs. throughput) — this decision, explained out loud, is worth more in an interview than the code implementing it
- In-memory hash index (`map[string]entryLoc`) wired to WAL offsets
- Put/Get/Delete against a single active file
- Startup recovery: replay the WAL from offset 0, rebuild the index, truncate a torn final record
- Unit tests: append-then-read round trip, checksum catches corruption, recovery rebuilds the index correctly
- Reference while building: `avinassh/go-caskdb` is the closest scope match; the Bitcask paper is about a 30-minute read

### Week 2 — Concurrency + robustness
- Decide the single-writer model — one goroutine owns writes via a channel/queue, vs. a mutex-guarded append — and document why, same as the fsync decision
- Concurrent readers while a write is in flight: get the ordering right (index update must follow durable write) and add a test that would catch it if the ordering were wrong
- Handle the "torn write at tail" case explicitly and test it — this is the single most common thing people get wrong, and the first thing an interviewer will probe
- Key deletion (tombstones) plus basic space accounting; leave actual space reclamation as a documented "future work" line rather than building a merge/compaction pass — that's LSM-tree scope creep

### Week 3 — Crash-recovery proof + benchmarks (the centerpiece)
This week produces the artifact that actually matters. The pattern that shows up across real crash-recovery suites is consistent:

1. Spawn the store as a **child process** (not in-process fault injection) — a real `kill -9` is the only way to exercise the actual process-death path (unflushed buffers, the kernel reaping the process), not just your own error-handling code paths
2. The child writes N records in a loop, printing each key only *after* the write call returns, so the parent only counts confirmed writes
3. The parent reads the child's stdout, waits for a target count or a randomized point, then `SIGKILL`s it
4. The parent reopens the store and verifies: every confirmed write survived, no corrupted entries, no phantom entries for writes that were never confirmed
5. Run this in a loop — 50 to 100 iterations with varied kill timing, not once. The first few runs prove the happy path; the later ones are what surface real bugs (the torn-write-at-tail case rarely shows up on run #1)

Benchmarks to record (these become the resume numbers):
- Writes/sec, single-threaded and with N concurrent writers
- p50 / p99 write latency
- Recovery time vs. WAL size (e.g., after 100K and 1M records)

### Week 4 — Stretch goal, or polish + buffer
If weeks 1–3 land on schedule, spend week 4 on the stretch goal:
- One leader, one follower. The leader appends locally, then ships each WAL record to the follower over TCP or gRPC; the follower appends to its own WAL and applies to its own index; the leader waits for a follower ack before acking the client — or doesn't, as long as that durability choice is written down
- Explicitly **not** building: leader election, quorum, term numbers — this is log shipping, not Raft, and the README should say so directly so nobody assumes more was built than was
- If the follower restarts, it should catch up from wherever its own WAL left off — that's the one piece of "recovery" logic worth testing here

If week 3 ran long — genuinely likely, since the crash-testing harness is the part most likely to eat extra time — use week 4 as buffer instead and skip the stretch goal. Cut the stretch goal before cutting crash-test rigor: a KV store with a clean recovery proof and no replication is a complete, credible project; one with half-working replication and a thin crash test is not.

Either way, week 4 closes with:
- A README covering the design decisions (fsync policy, single-writer model, hash index over B-tree, what recovery guarantees are and aren't provided), the record-format diagram, the benchmark numbers, and a short "how to reproduce the crash test" section
- The resume bullet (§6)

---

## 5. Testing checklist
- [ ] Append-then-read round trip
- [ ] Checksum catches a corrupted record
- [ ] Recovery truncates a torn write at the tail instead of crashing or silently dropping earlier data
- [ ] Concurrent writers don't corrupt the WAL or the index
- [ ] Kill-and-recover harness: 50+ runs, varied kill timing, zero data loss on confirmed writes
- [ ] Benchmarks recorded: writes/sec, p99 latency, recovery time per MB/records
- [ ] (Stretch) follower catches up correctly after its own restart

---

## 6. Resume bullet — draft
Fill the brackets in only once the numbers are real:

> Built a durable single-node key-value store in Go with an append-only write-ahead log and hash-indexed storage engine (Bitcask design); verified crash-safety via an automated kill-and-recover harness across [N]+ induced process kills with zero data loss on confirmed writes, sustaining [X] writes/sec at p99 [Y]ms and [Z]s recovery time per million records.

Don't ship this with placeholders still in it — an unverified metric is worse than no metric, and it's the easiest thing in the bullet for an interviewer to pull on.

---

## 7. Where this fits the target list
- **Oracle, Salesforce** — lead with this project; direct thematic match to what they build
- **Google, Amazon** — this is the answer if a system-design round asks "how would you build a key-value store"; keep FogCache/HookForge as the primary projects otherwise
- **Akamai, Adobe** — secondary mention at most; FogCache and HookForge stay the leads there respectively

---

## References
- Bitcask design, plain-language write-up: https://arpitbhayani.me/blogs/bitcask/
- Go reference implementations: https://github.com/avinassh/go-caskdb (closest scope match) · https://github.com/mr-karan/barreldb · https://github.com/SarthakMakhija/bitcask
- BoltDB, for contrast on why a B+tree store skips the WAL: https://github.com/boltdb/bolt
- Badger, if the stretch LSM path is taken instead: https://github.com/dgraph-io/badger
- Raft, for the replication stretch goal's mental model (not being implemented in full): "In Search of an Understandable Consensus Algorithm" (the Raft paper)
- A similar completed project, for testing-methodology reference only: https://github.com/jsav2003/durable-kv