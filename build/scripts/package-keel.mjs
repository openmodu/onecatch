// Package with the pinned Keel CLI. Release metadata is scoped to this build;
// the generated initializer and temporary config are always restored.
import { readFileSync, writeFileSync, unlinkSync, mkdirSync, rmSync, existsSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { parseChangelog } from './release-info.mjs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
process.chdir(fileURLToPath(new URL('../..', import.meta.url)));
const { version } = parseChangelog(readFileSync('CHANGELOG.md','utf8'));
const configPath = 'keel.json';
const original = readFileSync(configPath,'utf8');
const config = JSON.parse(original);
const generated = 'internal/platform/buildinfo/zz_release_generated.go';
const lock = 'build/keel/.package-lock';
mkdirSync(lock); // Fail if another packaging operation owns the temporary config.
let wroteGenerated = false;
function run(command, args) {
  const result = spawnSync(command,args,{stdio:'inherit'});
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} exited with ${result.status}`);
}
try {
  if (existsSync(generated)) throw new Error(`Refusing to overwrite ${generated}`);
  const publicKey = readFileSync('build/update/public-key.base64','utf8').trim();
  // Portable Windows/Linux releases require manual installation. Only bundled
  // macOS releases currently publish an automatic-update artifact.
  writeFileSync(generated,`package buildinfo\nfunc init() { Version = ${JSON.stringify(version)}; UpdatePublicKey = ${JSON.stringify(process.platform === 'darwin' ? publicKey : '')} }\n`,{flag:'wx'});
  wroteGenerated = true;
  config.version = version;
  writeFileSync(configPath,JSON.stringify(config,null,2)+'\n');
  const args = ['tool','keel','build','-o','bin/keel'];
  if (process.env.SIGN_IDENTITY) args.push('-sign',process.env.SIGN_IDENTITY);
  run('go',args);
  mkdirSync('dist',{recursive:true});
  const os = {darwin:'darwin',win32:'windows',linux:'linux'}[process.platform];
  const arch = {x64:'amd64',arm64:'arm64'}[process.arch];
  if (!os || !arch) throw new Error('Unsupported release platform');
  const basename = `OneCatch-${version}-${os}-${arch}`;
  if (os === 'darwin') {
    const bundle = `bin/keel/${config.name}.app`;
    run('go',['run','./build/icon','-optimize-icns',`${bundle}/Contents/Resources/icon.icns`]);
    const signArgs = ['--force','--deep','--sign',process.env.SIGN_IDENTITY || '-'];
    if (process.env.SIGN_IDENTITY) signArgs.push('--options','runtime','--timestamp');
    run('codesign',[...signArgs,bundle]);
    run('codesign',['--verify','--deep','--strict',bundle]);
    run('ditto',['-c','-k','--zlibCompressionLevel','9','--keepParent',bundle,`dist/${basename}.zip`]);
  } else if (os === 'windows') {
    const exe = path.resolve(`bin/keel/${config.binary}.exe`);
    const dest = path.resolve(`dist/${basename}.zip`);
    // Fixed project-controlled paths; single quotes escaped for PowerShell.
    const quote = value => `'${value.replaceAll("'","''")}'`;
    run('powershell',['-NoProfile','-Command',`Compress-Archive -LiteralPath ${quote(exe)} -DestinationPath ${quote(dest)} -Force`]);
  } else {
    run('tar',['-czf',`dist/${basename}.tar.gz`,'-C','bin/keel/linux','.']);
  }
} finally {
  writeFileSync(configPath,original);
  if (wroteGenerated) unlinkSync(generated);
  rmSync(lock,{recursive:true});
}
