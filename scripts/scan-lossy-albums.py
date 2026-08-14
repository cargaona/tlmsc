"""Survey every zero-FLAC album in the beets library against Deezer.

Read-only: opens library.db immutable, downloads nothing, writes one CSV to
/data/staging/flac-candidates.csv. Safe to re-run.
"""
import sqlite3, deezer, tomllib, unicodedata, re, time, csv, sys
from difflib import SequenceMatcher

ARL = tomllib.load(open('/tmp/streamrip-home/.config/streamrip/config.toml', 'rb'))['deezer']['arl']
dz = deezer.Deezer()
if not dz.login_via_arl(ARL):
    sys.exit('deezer login failed')

NOISE = re.compile(
    r'\b(deluxe|remaster(ed)?|edition|bonus|expanded|version|explicit|anniversary'
    r'|reissue|mono|stereo|vol|disc|cd|the)\b')

def norm(s):
    s = unicodedata.normalize('NFKD', s or '')
    s = s.replace('’', "'").replace('“', '"').replace('”', '"')
    s = s.encode('ascii', 'ignore').decode().lower()
    s = s.replace('&', ' and ')
    s = re.sub(r'\(.*?\)|\[.*?\]', ' ', s)
    s = NOISE.sub(' ', s)
    s = re.sub(r'[^a-z0-9]+', ' ', s)
    return ' '.join(s.split())

def sim(a, b):
    a, b = norm(a), norm(b)
    if not a or not b:
        return 0.0
    if a == b:
        return 1.0
    if a in b or b in a:
        return 0.93
    return SequenceMatcher(None, a, b).ratio()

con = sqlite3.connect('file:/mnt/seagate/music/library.db?immutable=1', uri=True)
albums = con.execute('''
  select a.id, a.albumartist, a.album, count(*) n,
         cast(avg(i.bitrate)/1000 as int) kbps,
         group_concat(distinct i.format)
  from albums a join items i on i.album_id = a.id
  group by a.id
  having sum(case when i.format="FLAC" then 1 else 0 end) = 0
  order by a.albumartist, a.album
''').fetchall()

print(f'scanning {len(albums)} zero-FLAC albums', flush=True)

out = open('/data/staging/flac-candidates.csv', 'w', newline='')
w = csv.writer(out)
w.writerow(['status', 'albumartist', 'album', 'local_tracks', 'local_kbps',
            'formats', 'deezer_id', 'deezer_title', 'deezer_artist',
            'deezer_tracks', 'flac_tracks', 'score', 'track_delta'])

tally = {'flac': 0, 'no_flac': 0, 'not_found': 0, 'error': 0}

for idx, (aid, artist, album, ntracks, kbps, formats) in enumerate(albums, 1):
    row = ['', artist, album, ntracks, kbps, formats, '', '', '', '', '', '', '']
    try:
        res = dz.api.search_album(f'{artist} {album}'.strip(), limit=8)
        cands = res.get('data', []) if isinstance(res, dict) else []

        best, best_score = None, 0.0
        for c in cands:
            s = 0.45 * sim(artist, (c.get('artist') or {}).get('name')) + 0.55 * sim(album, c.get('title'))
            if s > best_score:
                best, best_score = c, s

        if not best or best_score < 0.72:
            row[0] = 'not_found'
            tally['not_found'] += 1
        else:
            t = dz.gw.get_album_tracks(best['id'])
            tracks = t if isinstance(t, list) else t.get('data', [])
            flac_n = sum(1 for x in tracks if int(x.get('FILESIZE_FLAC') or 0) > 0)
            row[0] = 'flac' if flac_n else 'no_flac'
            tally['flac' if flac_n else 'no_flac'] += 1
            row[6:] = [best['id'], best.get('title'), (best.get('artist') or {}).get('name'),
                       len(tracks), flac_n, f'{best_score:.2f}', len(tracks) - ntracks]
    except Exception as e:
        row[0] = 'error'
        row[7] = str(e)[:80]
        tally['error'] += 1

    w.writerow(row)
    if idx % 50 == 0:
        out.flush()
        print(f'  {idx}/{len(albums)}  {tally}', flush=True)
    time.sleep(0.35)

out.close()
print('DONE', tally, flush=True)
