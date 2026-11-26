import { extractImageThumb } from '@whiskeysockets/baileys/lib/Utils/messages-media.js';
import { Jimp } from 'jimp';
const img = new Jimp({ width: 64, height: 64, color: 0xff0000ff });
const buf = await img.getBuffer('image/jpeg', { quality: 70 });
console.log('buffer bytes', buf.length);
try {
  const t = await extractImageThumb(buf, 32);
  console.log('THUMB OK', t.buffer.length, JSON.stringify(t.original));
} catch (e) {
  console.log('THUMB FAIL', e.message);
}
