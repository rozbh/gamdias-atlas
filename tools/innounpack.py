#!/usr/bin/env python3
"""Static unpacker for Inno Setup 6.5.x installers (tested on 6.5.2).

Never executes anything from the installer: it parses the setup header and
decompresses the embedded files.  Layouts follow Projects/Src/Shared.Struct.pas
from the public Inno Setup source (tag is-6_5_2).

usage: python3 -I innounpack.py <setup.exe> <outdir>
"""
import hashlib
import json
import lzma
import os
import re
import struct
import sys
import zlib

LDR_ID = b'rDlPtS\xcd\xe6\xd7\x7b\x0b\x2a'
CHUNK = 65536


class Reader:
    def __init__(self, data):
        self.d = data
        self.p = 0

    def take(self, n):
        if self.p + n > len(self.d):
            raise EOFError('read past end at %d (+%d of %d)' % (self.p, n, len(self.d)))
        b = self.d[self.p:self.p + n]
        self.p += n
        return b

    def u32(self):
        return struct.unpack('<I', self.take(4))[0]

    def i32(self):
        return struct.unpack('<i', self.take(4))[0]

    def wstr(self):
        return self.take(self.u32()).decode('utf-16-le', 'replace')

    def astr(self):
        return self.take(self.u32())


def read_block(f):
    """TCompressedBlockReader: CRC'd header, then 4096-byte CRC'd chunks of LZMA1."""
    hdr_crc, = struct.unpack('<I', f.read(4))
    hdr = f.read(5)
    if zlib.crc32(hdr) != hdr_crc:
        raise ValueError('block header CRC mismatch')
    stored, compressed = struct.unpack('<IB', hdr)
    raw = bytearray()
    left = stored
    while left > 0:
        crc, = struct.unpack('<I', f.read(4))
        left -= 4
        n = min(left, 4096)
        buf = f.read(n)
        left -= n
        if zlib.crc32(buf) != crc:
            raise ValueError('block chunk CRC mismatch')
        raw += buf
    if not compressed:
        return bytes(raw)
    return lzma1_decoder(bytes(raw[:5])).decompress(bytes(raw[5:]))


def lzma1_decoder(props):
    filt = lzma._decode_filter_properties(lzma.FILTER_LZMA1, props)
    return lzma.LZMADecompressor(lzma.FORMAT_RAW, filters=[filt])


def lzma2_decoder(prop):
    dict_size = (2 | (prop & 1)) << (prop // 2 + 11)
    return lzma.LZMADecompressor(
        lzma.FORMAT_RAW, filters=[{'id': lzma.FILTER_LZMA2, 'dict_size': dict_size}])


def ver(r):
    win, nt, sp = struct.unpack('<IIH', r.take(10))
    return {'win': '%d.%d.%d' % (win >> 24, (win >> 16) & 255, win & 65535),
            'nt': '%d.%d.%d' % (nt >> 24, (nt >> 16) & 255, nt & 65535),
            'sp': sp}


def bits(raw, names):
    v = int.from_bytes(raw, 'little')
    return [n for i, n in enumerate(names) if v >> i & 1]


HEADER_STRINGS = '''AppName AppVerName AppId AppCopyright AppPublisher AppPublisherURL
AppSupportPhone AppSupportURL AppUpdatesURL AppVersion DefaultDirName DefaultGroupName
BaseFilename UninstallFilesDir UninstallDisplayName UninstallDisplayIcon AppMutex
DefaultUserInfoName DefaultUserInfoOrg DefaultUserInfoSerial AppReadmeFile AppContact
AppComments AppModifyPath CreateUninstallRegKey Uninstallable CloseApplicationsFilter
SetupMutex ChangesEnvironment ChangesAssociations ArchitecturesAllowed
ArchitecturesInstallIn64BitMode CloseApplicationsFilterExcludes SevenZipLibraryName'''.split()
HEADER_COUNTS = '''Language CustomMessage Permission Type Component Task Dir ISSigKey File
FileLocation Icon Ini Registry InstallDelete UninstallDelete Run UninstallRun'''.split()
HEADER_OPTS = '''DisableStartupPrompt CreateAppDir AllowNoIcons AlwaysRestart
AlwaysUsePersonalGroup EnableDirDoesntExistWarning Password AllowRootDirectory
DisableFinishedPage UsePreviousAppDir UsePreviousGroup UpdateUninstallLogAppName
UsePreviousSetupType DisableReadyMemo AlwaysShowComponentsList FlatComponentsList
ShowComponentSizes UsePreviousTasks DisableReadyPage AlwaysShowDirOnReadyPage
AlwaysShowGroupOnReadyPage AllowUNCPath UserInfoPage UsePreviousUserInfo
UninstallRestartComputer RestartIfNeededByRun ShowTasksTreeLines AllowCancelDuringInstall
WizardImageStretch AppendDefaultDirName AppendDefaultGroupName SetupLogging
SignedUninstaller UsePreviousLanguage DisableWelcomePage CloseApplications
RestartApplications AllowNetworkDrive ForceCloseApplications AppNameHasConsts
UsePreviousPrivileges WizardResizable UninstallLogging'''.split()
FILE_OPTS = '''confirmoverwrite uninsneveruninstall restartreplace deleteafterinstall
regserver regtypelib sharedfile comparetimestamp fontisnttruetype skipifsourcedoesntexist
overwritereadonly overwritesameversion customdestname onlyifdestfileexists noregerror
uninsrestartdelete onlyifdoesntexist ignoreversion promptifolder dontcopy
uninsremovereadonly recursesubdirsexternal replacesameversion dontverifychecksum
uninsnosharedfileprompt createallsubdirs 32bit 64bit externalsizepreset setntfscompression
unsetntfscompression gacinstall download extractarchive'''.split()
LOC_FLAGS = ['VersionInfoValid', 'TimeStampInUTC', 'CallInstructionOptimized',
             'ChunkEncrypted', 'ChunkCompressed']
RUN_OPTS = '''shellexec skipifdoesntexist postinstall unchecked skipifsilent skipifnotsilent
hidewizard 32bit 64bit runasoriginaluser dontlogparameters logoutput'''.split()
REG_OPTS = '''createvalueifdoesntexist uninsdeletevalue uninsclearvalue uninsdeletekey
uninsdeletekeyifempty preservestringtype deletekey deletevalue noerror dontcreatekey
32bit 64bit'''.split()
REG_TYPES = ['none', 'string', 'expandsz', 'dword', 'binary', 'multisz', 'qword']
ROOTS = {0x80000000: 'HKCR', 0x80000001: 'HKCU', 0x80000002: 'HKLM', 0x80000003: 'HKU',
         0x80000005: 'HKCC'}
COMPRESS = ['stored', 'zip', 'bzip', 'lzma', 'lzma2']


def strs(r, names):
    return {n: r.wstr() for n in names}


def parse_header(r):
    h = strs(r, HEADER_STRINGS)
    ansi = {n: r.astr() for n in ('LicenseText', 'InfoBeforeText', 'InfoAfterText',
                                  'CompiledCodeText')}
    counts = dict(zip(HEADER_COUNTS, struct.unpack('<17i', r.take(68))))
    h['MinVersion'], h['OnlyBelowVersion'] = ver(r), ver(r)
    h['WizardStyle'] = r.take(1)[0]
    h['WizardSizePercent'] = struct.unpack('<ii', r.take(8))
    r.take(1)                      # WizardImageAlphaFormat
    r.take(8)                      # WizardImageBackColor, WizardSmallImageBackColor
    h['ExtraDiskSpaceRequired'], h['SlicesPerDisk'] = struct.unpack('<qi', r.take(12))
    (h['UninstallLogMode'], h['DirExistsWarning'], h['PrivilegesRequired'], _ovr,
     h['ShowLanguageDialog'], h['LanguageDetectionMethod'], cm,
     h['DisableDirPage'], h['DisableProgramGroupPage']) = r.take(9)
    h['PrivilegesRequired'] = ['none', 'poweruser', 'admin', 'lowest'][h['PrivilegesRequired']]
    h['CompressMethod'] = COMPRESS[cm]
    h['UninstallDisplaySize'], = struct.unpack('<q', r.take(8))
    h['Options'] = bits(r.take(6), HEADER_OPTS)
    return h, ansi, counts


COMMON = ['Components', 'Tasks', 'Languages', 'Check', 'AfterInstall', 'BeforeInstall']


def parse_entries(r, counts):
    e = {}
    e['languages'] = []
    for _ in range(counts['Language']):
        d = strs(r, ['Name', 'LanguageName', 'DialogFontName', 'TitleFontName',
                     'WelcomeFontName', 'CopyrightFontName'])
        d['_data'] = [r.astr() for _ in range(4)]
        d['LanguageID'] = r.u32()
        r.take(17)
        e['languages'].append(d)
    e['custom_messages'] = []
    for _ in range(counts['CustomMessage']):
        d = strs(r, ['Name', 'Value'])
        d['LangIndex'] = r.i32()
        e['custom_messages'].append(d)
    for _ in range(counts['Permission']):
        r.astr()
    e['types'] = []
    for _ in range(counts['Type']):
        e['types'].append(strs(r, ['Name', 'Description', 'Languages', 'CheckOnce']))
        r.take(30)
    e['components'] = []
    for _ in range(counts['Component']):
        e['components'].append(strs(r, ['Name', 'Description', 'Types', 'Languages', 'CheckOnce']))
        r.take(42)
    e['tasks'] = []
    for _ in range(counts['Task']):
        d = strs(r, ['Name', 'Description', 'GroupDescription', 'Components', 'Languages', 'Check'])
        r.take(25)
        d['Options'] = bits(r.take(1), ['exclusive', 'unchecked', 'restart', 'checkedonce',
                                        'dontinheritcheck'])
        e['tasks'].append(d)
    e['dirs'] = []
    for _ in range(counts['Dir']):
        d = strs(r, ['DirName'] + COMMON)
        r.take(27)
        e['dirs'].append(d)
    for _ in range(counts['ISSigKey']):
        strs(r, ['PublicX', 'PublicY', 'RuntimeID'])
    e['files'] = []
    for _ in range(counts['File']):
        d = strs(r, ['SourceFilename', 'DestName', 'InstallFontName', 'StrongAssemblyName']
                 + COMMON + ['Excludes', 'DownloadISSigSource', 'DownloadUserName',
                             'DownloadPassword', 'ExtractArchivePassword'])
        r.astr()                   # Verification.ISSigAllowedKeys
        r.take(33)                 # Verification.Hash, Verification.Typ
        r.take(20)                 # MinVersion, OnlyBelowVersion
        d['LocationEntry'], d['Attribs'], d['ExternalSize'] = struct.unpack('<iiq', r.take(16))
        r.take(2)                  # PermissionsEntry
        d['Options'] = bits(r.take(5), FILE_OPTS)
        d['FileType'] = ['user', 'uninstexe'][r.take(1)[0]]
        e['files'].append(d)
    e['icons'] = []
    for _ in range(counts['Icon']):
        d = strs(r, ['IconName', 'Filename', 'Parameters', 'WorkingDir', 'IconFilename',
                     'Comment'] + COMMON + ['AppUserModelID'])
        r.take(36)
        d['IconIndex'], d['ShowCmd'] = struct.unpack('<ii', r.take(8))
        r.take(4)
        e['icons'].append(d)
    e['ini'] = []
    for _ in range(counts['Ini']):
        e['ini'].append(strs(r, ['Filename', 'Section', 'Entry', 'Value'] + COMMON))
        r.take(21)
    e['registry'] = []
    for _ in range(counts['Registry']):
        d = strs(r, ['Subkey', 'ValueName', 'ValueData'] + COMMON)
        r.take(20)
        root, _perm, typ = struct.unpack('<IhB', r.take(7))
        d['Root'] = ROOTS.get(root | 0x80000000, hex(root))
        d['Type'] = REG_TYPES[typ] if typ < len(REG_TYPES) else typ
        d['Options'] = bits(r.take(2), REG_OPTS)
        e['registry'].append(d)
    for key, n in (('install_delete', counts['InstallDelete']),
                   ('uninstall_delete', counts['UninstallDelete'])):
        e[key] = []
        for _ in range(n):
            d = strs(r, ['Name'] + COMMON)
            r.take(20)
            d['Type'] = ['files', 'filesandordirs', 'dirifempty'][r.take(1)[0]]
            e[key].append(d)
    for key, n in (('run', counts['Run']), ('uninstall_run', counts['UninstallRun'])):
        e[key] = []
        for _ in range(n):
            d = strs(r, ['Name', 'Parameters', 'WorkingDir', 'RunOnceId', 'StatusMsg', 'Verb',
                         'Description'] + COMMON)
            r.take(20)
            d['ShowCmd'] = r.i32()
            d['Wait'] = ['waituntilterminated', 'nowait', 'waituntilidle'][r.take(1)[0]]
            d['Options'] = bits(r.take(2), RUN_OPTS)
            e[key].append(d)
    return e


def parse_locations(data, n):
    r = Reader(data)
    out = []
    for _ in range(n):
        (first, last, start, sub, size, csize) = struct.unpack('<iiqqqq', r.take(40))
        sha = r.take(32)
        ft, = struct.unpack('<Q', r.take(8))
        r.take(8)
        flags = bits(r.take(1), LOC_FLAGS)
        out.append({'FirstSlice': first, 'LastSlice': last, 'StartOffset': start,
                    'ChunkSuboffset': sub, 'OriginalSize': size, 'ChunkCompressedSize': csize,
                    'SHA256': sha.hex(), 'FileTime': ft, 'Flags': flags})
    return out


def untransform(buf, addr_offset):
    """Inverse of Inno's x86 CALL/JMP transform (TransformCallInstructions v3)."""
    size = len(buf) - 4
    if size < 1:
        return
    i = 0
    find = re.compile(b'[\xe8\xe9]').search
    while True:
        m = find(buf, i, size)
        if not m:
            return
        i = m.start() + 1
        if buf[i + 3] in (0x00, 0xff):
            addr = (addr_offset + i + 4) & 0xffffff
            rel = (buf[i] | buf[i + 1] << 8 | buf[i + 2] << 16) - addr & 0xffffffff
            if rel & 0x800000:
                buf[i + 3] ^= 0xff
            buf[i] = rel & 0xff
            buf[i + 1] = rel >> 8 & 0xff
            buf[i + 2] = rel >> 16 & 0xff
        i += 4


def safe_rel(dest):
    """Map an Inno DestName like {app}\\bin\\x.dll onto a contained relative path."""
    parts = []
    for part in dest.replace('/', '\\').split('\\'):
        part = part.strip()
        m = re.fullmatch(r'\{(\w+)[^}]*\}', part)
        if m:
            part = m.group(1)
        part = re.sub(r'[<>:"|?*{}\x00-\x1f]', '_', part)
        if part in ('', '.', '..'):
            continue
        parts.append(part)
    return os.path.join(*parts) if parts else '_unnamed'


class ChunkStream:
    """Sequential reader over one decompressed data chunk."""

    def __init__(self, f, base, loc, method):
        f.seek(base + loc['StartOffset'])
        if f.read(4) != b'zlb\x1a':
            raise ValueError('bad chunk id at %d' % loc['StartOffset'])
        self.f = f
        self.left = loc['ChunkCompressedSize']
        self.pos = 0
        self.buf = bytearray()
        self.dec = None
        if 'ChunkCompressed' in loc['Flags']:
            if method == 'lzma2':
                self.dec = lzma2_decoder(self._raw(1)[0])
            elif method == 'lzma':
                self.dec = lzma1_decoder(self._raw(5))
            else:
                raise ValueError('unsupported compression: ' + method)

    def _raw(self, n):
        n = min(n, self.left)
        self.left -= n
        return self.f.read(n)

    def read(self, n):
        while len(self.buf) < n:
            raw = self._raw(1 << 20)
            if not raw:
                break
            self.buf += self.dec.decompress(raw) if self.dec else raw
        out = bytes(self.buf[:n])
        del self.buf[:n]
        self.pos += len(out)
        return out

    def skip(self, n):
        while n > 0:
            got = len(self.read(min(n, 1 << 20)))
            if not got:
                raise EOFError('chunk ended while seeking')
            n -= got


def main():
    exe, outdir = sys.argv[1], sys.argv[2]
    os.makedirs(outdir, exist_ok=True)
    meta_dir = os.path.join(outdir, '_installer')
    os.makedirs(meta_dir, exist_ok=True)
    f = open(exe, 'rb')

    head = f.read(4 << 20)
    at = head.find(LDR_ID)
    if at < 0:
        sys.exit('SetupLdr offset table not found')
    (version, total, off_exe, usize_exe, _crc_exe, off0, off1, _pad,
     table_crc) = struct.unpack_from('<IqqIiqqII', head, at + 12)
    if zlib.crc32(head[at:at + 60]) != table_crc:
        sys.exit('offset table CRC mismatch')
    print('offset table v%d: setup-0 @%d, setup-1 @%d' % (version, off0, off1))

    f.seek(off0)
    setup_id = f.read(64).rstrip(b'\0').decode('latin-1')
    print('setup id:', setup_id)
    enc_crc, = struct.unpack('<I', f.read(4))
    enc = f.read(49)
    if zlib.crc32(enc) != enc_crc:
        sys.exit('encryption header CRC mismatch (layout differs from 6.5.x?)')
    if enc[0] != 0:
        sys.exit('installer is encrypted (EncryptionUse=%d); not supported' % enc[0])

    r = Reader(read_block(f))
    header, ansi, counts = parse_header(r)
    entries = parse_entries(r, counts)
    images = []
    for kind in ('wizard', 'wizard_small'):
        for i in range(r.i32()):
            data = r.take(r.i32())
            ext = 'png' if data[:4] == b'\x89PNG' else 'bmp'
            name = '%s_%d.%s' % (kind, i, ext)
            open(os.path.join(meta_dir, name), 'wb').write(data)
            images.append(name)
    if header['CompressMethod'] in ('zip', 'bzip'):
        r.take(r.i32())
    if header['SevenZipLibraryName']:
        r.take(r.i32())
    print('header parsed, %d trailing bytes' % (len(r.d) - r.p))
    locs = parse_locations(read_block(f), counts['FileLocation'])

    for name, blob in ansi.items():
        if blob:
            open(os.path.join(meta_dir, name + '.bin'), 'wb').write(blob)
    for lang in entries['languages']:
        blobs = lang.pop('_data')
        if blobs[0]:
            open(os.path.join(meta_dir, 'lang_%s.isl.bin' % lang['Name']), 'wb').write(blobs[0])

    # Several file entries may share one location (identical content).
    by_loc = {}
    for fe in entries['files']:
        if fe['LocationEntry'] >= 0:
            by_loc.setdefault(fe['LocationEntry'], []).append(fe)
    chunks = {}
    for idx, loc in enumerate(locs):
        chunks.setdefault((loc['FirstSlice'], loc['StartOffset']), []).append(idx)

    used, manifest, bad = set(), [], 0
    for key in sorted(chunks):
        idxs = sorted(chunks[key], key=lambda i: locs[i]['ChunkSuboffset'])
        stream = ChunkStream(f, off1, locs[idxs[0]], header['CompressMethod'])
        for idx in idxs:
            loc = locs[idx]
            if 'ChunkEncrypted' in loc['Flags']:
                sys.exit('encrypted chunk; not supported')
            stream.skip(loc['ChunkSuboffset'] - stream.pos)
            targets = []
            for fe in by_loc.get(idx, [{'DestName': '_orphan\\loc_%d.bin' % idx}]):
                rel = safe_rel(fe['DestName'] if fe.get('FileType') != 'uninstexe'
                               else '_installer\\uninstall_stub.e32')
                base, n = rel, 1
                while rel in used:
                    root, ext = os.path.splitext(base)
                    rel = '%s~%d%s' % (root, n, ext)
                    n += 1
                used.add(rel)
                targets.append((rel, fe))
            path0 = os.path.join(outdir, targets[0][0])
            os.makedirs(os.path.dirname(path0), exist_ok=True)
            sha = hashlib.sha256()
            left, addr = loc['OriginalSize'], 0
            with open(path0, 'wb') as out:
                while left > 0:
                    buf = bytearray(stream.read(min(left, CHUNK)))
                    if not buf:
                        raise EOFError('chunk ended inside ' + targets[0][0])
                    if 'CallInstructionOptimized' in loc['Flags']:
                        untransform(buf, addr)
                        addr = addr + len(buf) & 0xffffffff
                    sha.update(buf)
                    out.write(buf)
                    left -= len(buf)
            ok = sha.hexdigest() == loc['SHA256']
            bad += not ok
            for rel, fe in targets[1:]:
                path = os.path.join(outdir, rel)
                os.makedirs(os.path.dirname(path), exist_ok=True)
                try:
                    os.link(path0, path)
                except OSError:
                    with open(path0, 'rb') as a, open(path, 'wb') as b:
                        b.write(a.read())
            for rel, fe in targets:
                manifest.append({'path': rel, 'dest': fe['DestName'],
                                 'size': loc['OriginalSize'], 'sha256': loc['SHA256'],
                                 'sha256_ok': ok, 'options': fe.get('Options', []),
                                 'check': fe.get('Check', ''), 'tasks': fe.get('Tasks', ''),
                                 'components': fe.get('Components', '')})

    info = {'setup_id': setup_id, 'header': header, 'counts': counts,
            'wizard_images': images, **{k: v for k, v in entries.items() if k != 'files'}}
    json.dump(info, open(os.path.join(meta_dir, 'setup_info.json'), 'w'),
              indent=1, ensure_ascii=False)
    json.dump(manifest, open(os.path.join(meta_dir, 'files.json'), 'w'),
              indent=1, ensure_ascii=False)
    print('extracted %d files (%d bytes), %d hash mismatches'
          % (len(manifest), sum(m['size'] for m in manifest), bad))
    return 1 if bad else 0


if __name__ == '__main__':
    sys.exit(main())
