const fs = require('fs');

const rtfToTxt = (rtf) => {
  return rtf
    .replace(/\{\\field[\s\S]*?\}\}/g, '')
    .replace(/\{\\fonttbl[\s\S]*?\}/g, '')
    .replace(/\{\\colortbl[\s\S]*?\}/g, '')
    .replace(/\{\\\*\\generator[\s\S]*?\}/g, '')
    .replace(/\\'[0-9a-fA-F]{2}/g, (m) =>
      String.fromCharCode(parseInt(m.slice(2), 16))
    )
    .replace(/\\par[d]?/g, '\n')
    .replace(/\\line/g, '\n')
    .replace(/\\[a-zA-Z]+\d* ?/g, '')
    .replace(/[{}]/g, '')
    .replace(/\n\s+\n/g, '\n\n')
    .trim();
};

const convertFile = (input, output) => {
  const rtf = fs.readFileSync(input, 'utf8');
  const txt = rtfToTxt(rtf);
  fs.writeFileSync(output, txt);
};

const [,, input, output] = process.argv;

if (!input || !output) {
  console.log('Usage: node rtf-to-txt.js <input.rtf> <output.txt>');
  process.exit(1);
}

convertFile(input, output);
