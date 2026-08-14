"""Replace lossy albums in the beets library with FLAC from Deezer.

The originals are moved to quarantine BEFORE the import runs. That ordering is
not cosmetic: beets' `duplicate_action: remove` deletes the duplicate's files
itself, so anything still sitting in the library when `beet import` runs is
destroyed rather than preserved. Getting the old copy out of the way first is
the only way to keep it.

  1. record the old album's file paths from the beets DB
  2. download the Deezer album to staging, verify it really is FLAC
  3. move the old files to quarantine  <- originals now safe from beets
  4. drop the old rows from the library DB
  5. beet import the new copy and verify it landed as FLAC
  6. on any failure in 4-5, put the old files back and re-import them

State lives on the quarantine volume, so a restart resumes where it stopped.

  python3 replace.py --dry-run          # plan only, touches nothing
  python3 replace.py --limit 5          # do 5 albums
  python3 replace.py                    # the lot
"""
import argparse, json, os, shutil, sqlite3, subprocess, sys, time, traceback

MUSIC = '/mnt/seagate/music'
QUAR = '/mnt/seagate/quarantine'
STAGING = '/data/staging'
LIB = f'{MUSIC}/library.db'
STATE = f'{QUAR}/.replace-state.json'
LOG = f'{QUAR}/replace.log'


def log(msg):
    line = f'{time.strftime("%Y-%m-%d %H:%M:%S")}  {msg}'
    print(line, flush=True)
    with open(LOG, 'a') as fh:
        fh.write(line + '\n')


def load_state():
    if os.path.exists(STATE):
        return json.load(open(STATE))
    return {'done': [], 'failed': {}, 'bytes': 0}


def save_state(st):
    tmp = STATE + '.tmp'
    json.dump(st, open(tmp, 'w'), indent=1)
    os.replace(tmp, STATE)


def db():
    # Not immutable: beet import writes here between our reads.
    return sqlite3.connect(LIB)


def find_album(con, artist, album):
    """Locate the beets album row. Returns (id, [paths]) or None."""
    row = con.execute(
        'select id from albums where albumartist=? and album=?', (artist, album)
    ).fetchone()
    if not row:
        return None
    aid = row[0]
    paths = []
    for (p,) in con.execute('select path from items where album_id=?', (aid,)):
        s = p.decode('utf-8', 'surrogateescape') if isinstance(p, bytes) else p
        # This library stores item paths RELATIVE to the beets directory, not
        # absolute as beets usually does. Treating them as absolute makes every
        # existence check silently fail, so quarantine would move nothing.
        paths.append(s if os.path.isabs(s) else os.path.join(MUSIC, s))
    return aid, paths


def staging_dirs():
    try:
        return {d for d in os.listdir(STAGING) if os.path.isdir(f'{STAGING}/{d}')}
    except FileNotFoundError:
        return set()


def flac_count(path):
    n = 0
    for root, _, files in os.walk(path):
        n += sum(1 for f in files if f.lower().endswith('.flac'))
    return n


def dir_bytes(path):
    total = 0
    for root, _, files in os.walk(path):
        for f in files:
            try:
                total += os.path.getsize(os.path.join(root, f))
            except OSError:
                pass
    return total


def run(cmd, timeout):
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)


RIP_CONFIG = '/tmp/streamrip-home/.config/streamrip/config.toml'


def disable_download_dedup():
    """Turn off streamrip's downloads database for the duration of the job.

    It records every track id it has ever fetched and silently skips them on a
    later run, emitting an empty directory instead. After any interrupted
    download that album becomes permanently unfetchable until the db is
    cleared -- which looks exactly like "Deezer has no FLAC". We always want a
    real download here, so the dedup is pure downside.

    entrypoint.sh regenerates this config on every pod restart, so this has to
    run each time rather than being a one-off edit.
    """
    try:
        txt = open(RIP_CONFIG).read()
    except FileNotFoundError:
        log('WARN  streamrip config not found; cannot disable download dedup')
        return
    changed = False
    if 'downloads_enabled = true' in txt:
        txt = txt.replace('downloads_enabled = true', 'downloads_enabled = false', 1)
        changed = True
        log('disabled streamrip download dedup (downloads_enabled = false)')

    # Six parallel FLAC connections plus a concurrent beets import is what
    # OOMKilled the container at the old 512Mi limit. Three keeps peak memory
    # well under the ceiling at negligible cost to throughput, since the
    # bottleneck here is Deezer rather than local bandwidth.
    if 'max_connections = 6' in txt:
        txt = txt.replace('max_connections = 6', 'max_connections = 3', 1)
        changed = True
        log('reduced streamrip max_connections to 3')

    if changed:
        open(RIP_CONFIG, 'w').write(txt)


def quarantine(paths):
    """Move old files under QUAR, preserving their layout below MUSIC.

    Returns the list of (original, quarantined) pairs so a failed import can
    be rolled back.
    """
    moved = []
    for p in paths:
        if not os.path.exists(p):
            continue
        rel = os.path.relpath(p, MUSIC)
        dest = os.path.join(QUAR, rel)
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        try:
            os.rename(p, dest)      # same filesystem: instant
        except OSError:
            shutil.move(p, dest)    # fallback if that ever stops being true
        moved.append((p, dest))
    # Prune album dirs left empty behind us.
    for p in paths:
        d = os.path.dirname(p)
        try:
            if os.path.isdir(d) and not os.listdir(d):
                os.rmdir(d)
        except OSError:
            pass
    return moved


def unquarantine(moved):
    """Undo quarantine() after a failed replacement."""
    for orig, dest in moved:
        if not os.path.exists(dest):
            continue
        os.makedirs(os.path.dirname(orig), exist_ok=True)
        try:
            os.rename(dest, orig)
        except OSError:
            shutil.move(dest, orig)


def process(entry, args, st):
    artist, album = entry['albumartist'], entry['album']
    dz_id, want = entry['deezer_id'], int(entry['local_tracks'])
    key = f'{artist}|||{album}'
    label = f'{artist} - {album}'

    con = db()
    found = find_album(con, artist, album)
    if not found:
        con.close()
        log(f'SKIP  {label}  (no longer in library)')
        st['failed'][key] = 'not in library'
        return False
    old_id, old_paths = found
    con.close()

    if args.dry_run:
        log(f'DRY   {label}  deezer={dz_id}  would replace {len(old_paths)} files')
        return True

    before = staging_dirs()

    # ---- download -------------------------------------------------------
    r = run(['rip', 'id', 'deezer', 'album', str(dz_id)], timeout=args.dl_timeout)
    new = staging_dirs() - before
    if not new:
        log(f'FAIL  {label}  download produced nothing'
            f'{" | " + r.stderr.strip()[-160:] if r.stderr.strip() else ""}')
        st['failed'][key] = 'download produced no directory'
        return False

    newdir = f'{STAGING}/{sorted(new)[0]}'
    got = flac_count(newdir)
    if got == 0:
        log(f'FAIL  {label}  downloaded but no FLAC files present')
        st['failed'][key] = 'no flac in download'
        shutil.rmtree(newdir, ignore_errors=True)
        return False
    if got < want - 2:
        log(f'FAIL  {label}  only {got} FLAC vs {want} local tracks - refusing')
        st['failed'][key] = f'track shortfall {got}<{want}'
        shutil.rmtree(newdir, ignore_errors=True)
        return False

    size = dir_bytes(newdir)

    # ---- retire the old copy BEFORE importing ---------------------------
    # beets deletes duplicate files during import (duplicate_action: remove),
    # so the originals have to be out of the library tree by now or they are
    # gone for good.
    moved = quarantine(old_paths)
    if old_paths and not moved:
        # Every original was already missing from disk. Something is wrong with
        # our view of the library (bad path resolution, files moved out of band)
        # and continuing would import the FLAC while silently leaving nothing
        # preserved. Refuse rather than destroy what we cannot see.
        log(f'FAIL  {label}  none of {len(old_paths)} originals found on disk - refusing to replace')
        st['failed'][key] = 'originals not found on disk'
        shutil.rmtree(newdir, ignore_errors=True)
        return False

    r = run(['beet', 'remove', '-a', '-f', f'id:{old_id}'], timeout=300)
    if r.returncode != 0:
        log(f'WARN  {label}  "beet remove" rc={r.returncode} | {(r.stderr or "").strip()[-120:]}')

    # ---- import ---------------------------------------------------------
    r = run(['beet', 'import', '-q', '--quiet-fallback=asis', newdir], timeout=args.import_timeout)
    import_ok = r.returncode == 0

    # ---- verify the library really gained a FLAC copy --------------------
    flac_n = 0
    if import_ok:
        con = db()
        rows = con.execute('''
            select a.id, sum(case when i.format="FLAC" then 1 else 0 end)
            from albums a join items i on i.album_id=a.id
            where a.albumartist=? and a.album=? group by a.id
        ''', (artist, album)).fetchall()
        con.close()
        flac_n = max([r[1] for r in rows], default=0)

    if not import_ok or flac_n == 0:
        why = f'import rc={r.returncode}' if not import_ok else 'no FLAC album after import'
        log(f'FAIL  {label}  {why} - rolling back {len(moved)} files')
        unquarantine(moved)
        rb = run(['beet', 'import', '-q', '--quiet-fallback=asis',
                  os.path.dirname(old_paths[0])], timeout=args.import_timeout) if old_paths else None
        if rb is not None and rb.returncode != 0:
            log(f'WARN  {label}  restored files but re-import rc={rb.returncode} - run "beet update"')
        st['failed'][key] = why
        shutil.rmtree(newdir, ignore_errors=True)
        return False

    shutil.rmtree(newdir, ignore_errors=True)
    st['done'].append(key)
    st['bytes'] += size
    log(f'OK    {label}  {got} FLAC in, {len(moved)} old files quarantined, {size/1048576:.0f} MB')
    return True


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--worklist', default=f'{QUAR}/worklist.json')
    ap.add_argument('--limit', type=int, default=0)
    ap.add_argument('--dry-run', action='store_true')
    ap.add_argument('--dl-timeout', type=int, default=1800)
    ap.add_argument('--import-timeout', type=int, default=900)
    ap.add_argument('--sleep', type=float, default=2.0)
    args = ap.parse_args()

    os.makedirs(QUAR, exist_ok=True)
    if not args.dry_run:
        disable_download_dedup()
    work = json.load(open(args.worklist))
    st = load_state()
    done = set(st['done'])

    todo = [e for e in work if f"{e['albumartist']}|||{e['album']}" not in done]
    if args.limit:
        todo = todo[:args.limit]

    log(f'=== start: {len(todo)} to process, {len(done)} already done '
        f'{"(DRY RUN)" if args.dry_run else ""} ===')

    ok = fail = 0
    for n, entry in enumerate(todo, 1):
        try:
            if process(entry, args, st):
                ok += 1
            else:
                fail += 1
        except subprocess.TimeoutExpired:
            fail += 1
            log(f'FAIL  {entry["albumartist"]} - {entry["album"]}  timed out')
            st['failed'][f"{entry['albumartist']}|||{entry['album']}"] = 'timeout'
        except Exception:
            fail += 1
            log(f'FAIL  {entry["albumartist"]} - {entry["album"]}\n{traceback.format_exc()[-400:]}')
        if not args.dry_run:
            save_state(st)
        if n % 10 == 0:
            log(f'--- {n}/{len(todo)}  ok={ok} fail={fail}  {st["bytes"]/1073741824:.1f} GB ---')
        time.sleep(args.sleep)

    save_state(st)
    log(f'=== finished: ok={ok} fail={fail}  total {st["bytes"]/1073741824:.1f} GB ===')


if __name__ == '__main__':
    main()
