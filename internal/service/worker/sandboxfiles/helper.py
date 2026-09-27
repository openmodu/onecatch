# Private one-session file RPC. Runs independently of Codex; requires Python 3
# and Git. No listener, token, or persistent executable is installed.
import base64, fcntl, hashlib, json, os, stat, subprocess, sys, uuid

LIMIT = 64 * 1024 * 1024
CHUNK = 192 * 1024
root = None
lock = None
pending = None
blocked = set()


def parts(name):
    if not isinstance(name, str) or not name or '\\' in name or '\x00' in name:
        raise ValueError('invalid path')
    p = name.split('/')
    if any(x in ('', '.', '..') or x.lower() in blocked or x.lower().startswith('.env')
           or x.lower().endswith(('.pem', '.key', '.p12', '.pfx')) or x.lower().startswith('.onecatch-') for x in p):
        raise ValueError('excluded path')
    return p


def parent(name, create=False):
    p = parts(name)
    fd = os.dup(root)
    try:
        for component in p[:-1]:
            if create:
                try: os.mkdir(component, 0o755, dir_fd=fd)
                except FileExistsError: pass
            nxt = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = nxt
        return fd, p[-1]
    except BaseException:
        os.close(fd)
        raise


def entry(name):
    try:
        fd, leaf = parent(name)
    except FileNotFoundError:
        return None
    try:
        try: f = os.open(leaf, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        except FileNotFoundError: return None
        with os.fdopen(f, 'rb') as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode): raise ValueError('not a regular file')
            if info.st_size > LIMIT: raise ValueError('file exceeds 64 MiB')
            h = hashlib.sha256()
            size = 0
            while True:
                data = stream.read(CHUNK)
                if not data: break
                size += len(data)
                if size > LIMIT: raise ValueError('file exceeds 64 MiB')
                h.update(data)
            after = os.fstat(stream.fileno())
            if (info.st_mtime_ns, info.st_size) != (after.st_mtime_ns, after.st_size):
                raise ValueError('file changed while reading')
            return {'hash': h.hexdigest(), 'size': size, 'executable': bool(info.st_mode & 0o111)}
    finally:
        os.close(fd)


def safe_dir(absolute):
    if not absolute.startswith('/'): raise ValueError('absolute path required')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for component in absolute.split('/')[1:]:
            if not component or component in ('.', '..'): raise ValueError('invalid root')
            try: os.mkdir(component, 0o700, dir_fd=fd)
            except FileExistsError: pass
            nxt = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = nxt
        return fd
    except BaseException:
        os.close(fd)
        raise


def run(req):
    global root, lock, pending, blocked
    op = req['op']
    if op == 'init':
        if root is not None: raise ValueError('already initialized')
        blocked = set(req['blocked'])
        root = safe_dir(req['root'])
        os.fchdir(root)
        lock = os.open('.onecatch-sync-lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600, dir_fd=root)
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        marker = '.onecatch-sync-generation'
        try:
            generation_fd = os.open(marker, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=root)
            with os.fdopen(generation_fd, 'w') as stream: stream.write(uuid.uuid4().hex)
        except FileExistsError: pass
        generation_fd = os.open(marker, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=root)
        with os.fdopen(generation_fd) as stream:
            if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode): raise ValueError('invalid generation marker')
            generation = stream.read(33)
        if len(generation) != 32 or any(x not in '0123456789abcdef' for x in generation): raise ValueError('invalid generation marker')
        # A private Git index is only used to evaluate nested .gitignore rules.
        # Keep it outside the agent workspace so the agent cannot change hooks
        # or core.fsmonitor used by this helper.
        return {'generation': generation}
    if root is None: raise ValueError('not initialized')
    if op == 'manifest':
        import tempfile
        with tempfile.TemporaryDirectory(prefix='onecatch-index-') as index:
            env = {**os.environ, 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'}
            for key in list(env):
                if key.startswith('GIT_') and key not in ('GIT_CONFIG_NOSYSTEM', 'GIT_CONFIG_GLOBAL'): del env[key]
            subprocess.run(['git', 'init', '--bare', '--template=', index], env=env, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            names = subprocess.check_output(['git', '--git-dir='+index, '--work-tree=.', '-c', 'core.bare=false', 'ls-files', '--others', '--exclude-standard', '-z'], env=env).decode('utf-8').split('\x00')
        names = set(names) | set(req.get('tracked', []))
        result = {}
        for name in sorted(names):
            try: parts(name)
            except ValueError: continue
            # Skip symlinks and special files, including symlink parents.
            try:
                fd, leaf = parent(name)
                try: info = os.stat(leaf, dir_fd=fd, follow_symlinks=False)
                finally: os.close(fd)
            except FileNotFoundError: continue
            except NotADirectoryError:
                if name in req.get('tracked', []): raise ValueError('synced parent replaced with a symlink or file')
                continue
            if not stat.S_ISREG(info.st_mode):
                if name in req.get('tracked', []): raise ValueError('synced file replaced with a non-regular file')
                continue
            item = entry(name)
            if item: result[name] = item
            if len(result) > 100000: raise ValueError('too many files')
        return {'files': result}
    if op == 'batch':
        uploads = req['uploads']
        if len(uploads) > 64 or sum(len(x['data']) for x in uploads) > CHUNK * 2: raise ValueError('batch too large')
        for upload in uploads:
            run({'op': 'begin', 'path': upload['path'], 'file': upload['file'], 'before': upload['before']})
            run({'op': 'chunk', 'path': upload['path'], 'data': upload['data']})
            run({'op': 'finish', 'path': upload['path']})
        return {}
    name = req.get('path', '')
    parts(name)
    if op == 'get':
        fd, leaf = parent(name)
        try: f = os.open(leaf, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        finally: os.close(fd)
        with os.fdopen(f, 'rb') as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_size > LIMIT: raise ValueError('invalid file')
            offset = req['offset']
            if offset < 0 or offset > LIMIT: raise ValueError('invalid offset')
            stream.seek(offset)
            return {'data': base64.b64encode(stream.read(CHUNK)).decode()}
    if op == 'begin':
        if pending is not None: raise ValueError('upload already open')
        fd, leaf = parent(name, True)
        temp = '.onecatch-upload-' + uuid.uuid4().hex
        try:
            f = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=fd)
        except BaseException:
            os.close(fd)
            raise
        pending = (fd, leaf, temp, os.fdopen(f, 'wb'), req, hashlib.sha256())
        return {}
    if op == 'chunk':
        if pending is None or pending[4]['path'] != name: raise ValueError('upload missing')
        data = base64.b64decode(req['data'], validate=True)
        if len(data) > CHUNK or pending[3].tell() + len(data) > LIMIT: raise ValueError('upload too large')
        pending[3].write(data)
        pending[5].update(data)
        return {}
    if op == 'finish':
        if pending is None or pending[4]['path'] != name: raise ValueError('upload missing')
        fd, leaf, temp, stream, begin, digest = pending
        expected = begin['file']
        if stream.tell() != expected['size'] or digest.hexdigest() != expected['hash']: raise ValueError('hash mismatch')
        if entry(name) != begin.get('before'): raise ValueError('remote file changed')
        os.fchmod(stream.fileno(), 0o755 if expected['executable'] else 0o644)
        stream.flush()
        os.fsync(stream.fileno())
        stream.close()
        os.rename(temp, leaf, src_dir_fd=fd, dst_dir_fd=fd)
        os.close(fd)
        pending = None
        return {}
    if op == 'delete':
        if entry(name) != req.get('before'): raise ValueError('remote file changed')
        fd, leaf = parent(name)
        try: os.unlink(leaf, dir_fd=fd)
        finally: os.close(fd)
        return {}
    raise ValueError('unknown operation')


try:
    for line in sys.stdin:
        try:
            if len(line) > 1024 * 1024 * 16: raise ValueError('request too large')
            response = run(json.loads(line))
        except Exception as e:
            # Do not leak environment or credential-bearing exception strings.
            response = {'error': type(e).__name__ + ': ' + (str(e) if isinstance(e, ValueError) else 'file operation failed')}
        print(json.dumps(response, separators=(',', ':')), flush=True)
finally:
    if pending:
        fd, leaf, temp, stream, begin, digest = pending
        stream.close()
        try: os.unlink(temp, dir_fd=fd)
        except FileNotFoundError: pass
        os.close(fd)
