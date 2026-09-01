#!/usr/bin/env node

// Angular component tree generator v2.

import { promises as fs } from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const COLORS = {
  blue: '\u001b[34m',
  cyan: '\u001b[36m',
  dim: '\u001b[2m',
  green: '\u001b[32m',
  magenta: '\u001b[35m',
  reset: '\u001b[0m',
  yellow: '\u001b[33m',
};

const ASSOCIATED_EXTENSIONS = [
  '.html',
  '.scss',
  '.css',
  '.sass',
  '.less',
  '.spec.ts',
];

const parseArguments = (argumentsList) => {
  const options = {
    color: Boolean(process.stdout.isTTY && !process.env.NO_COLOR),
    output: null,
    root: 'src/app',
    unicode: false,
  };

  for (let index = 0; index < argumentsList.length; index += 1) {
    const argument = argumentsList[index];

    if (argument === '--no-color') {
      options.color = false;
    } else if (argument === '--unicode') {
      options.unicode = true;
    } else if (argument === '--output' || argument === '-o') {
      options.output = argumentsList[index + 1];
      index += 1;
    } else if (!argument.startsWith('-')) {
      options.root = argument;
    } else {
      throw new Error(`OpciÃ³n desconocida: ${argument}`);
    }
  }

  if (options.output && !path.extname(options.output)) {
    throw new Error('El archivo de salida debe tener una extensiÃ³n, por ejemplo .txt');
  }

  return options;
};

const paint = (enabled, color, value) =>
  enabled ? `${COLORS[color]}${value}${COLORS.reset}` : value;

const walk = async (directory) => {
  const entries = await fs.readdir(directory, { withFileTypes: true });
  const nestedFiles = await Promise.all(
    entries
      .filter((entry) => !entry.name.startsWith('.') && entry.name !== 'node_modules')
      .map((entry) => {
        const absolutePath = path.join(directory, entry.name);
        return entry.isDirectory() ? walk(absolutePath) : [absolutePath];
      }),
  );

  return nestedFiles.flat();
};

const firstMatch = (source, expression, fallback = null) =>
  source.match(expression)?.[1]?.trim() ?? fallback;

const listMatches = (source, expression) =>
  [...source.matchAll(expression)].map((match) => match[1].trim());

const normalizeComponentStem = (filename) =>
  filename.replace(/\.component\.ts$/, '').replace(/\.ts$/, '');

const findAssociatedFiles = async (typescriptPath, source) => {
  const directory = path.dirname(typescriptPath);
  const filename = path.basename(typescriptPath);
  const stem = normalizeComponentStem(filename);
  const referencedNames = [
    firstMatch(source, /templateUrl\s*:\s*['"`]([^'"`]+)['"`]/),
    ...listMatches(source, /styleUrl\s*:\s*['"`]([^'"`]+)['"`]/g),
  ].filter(Boolean);

  const styleUrlsBlock = firstMatch(source, /styleUrls\s*:\s*\[([\s\S]*?)\]/);
  if (styleUrlsBlock) {
    referencedNames.push(...listMatches(styleUrlsBlock, /['"`]([^'"`]+)['"`]/g));
  }

  const conventionalNames = ASSOCIATED_EXTENSIONS.flatMap((extension) => [
    `${stem}${extension}`,
    `${stem}.component${extension}`,
  ]);

  const candidates = [...new Set([...referencedNames, ...conventionalNames])];
  const existingFiles = await Promise.all(
    candidates.map(async (candidate) => {
      const candidatePath = path.resolve(directory, candidate);
      try {
        const stat = await fs.stat(candidatePath);
        return stat.isFile() ? candidatePath : null;
      } catch {
        return null;
      }
    }),
  );

  return [...new Set([typescriptPath, ...existingFiles.filter(Boolean)])];
};

const inspectComponent = async (typescriptPath, rootPath) => {
  const source = await fs.readFile(typescriptPath, 'utf8');
  if (!/@Component\s*\(/.test(source)) return null;

  const className = firstMatch(
    source,
    /export\s+(?:default\s+)?class\s+([A-Za-z_$][\w$]*)/,
    normalizeComponentStem(path.basename(typescriptPath)),
  );
  const selector = firstMatch(source, /selector\s*:\s*['"`]([^'"`]+)['"`]/, 'sin selector');
  // Angular 19+ uses standalone components by default. Only an explicit
  // `standalone: false` means that the component belongs to an NgModule.
  const standalone = !/standalone\s*:\s*false/.test(source);
  const files = await findAssociatedFiles(typescriptPath, source);

  return {
    className,
    directory: path.relative(rootPath, path.dirname(typescriptPath)),
    files,
    selector,
    standalone,
  };
};

const createDirectoryNode = (name = '') => ({
  children: new Map(),
  components: [],
  name,
});

const buildTree = (components) => {
  const root = createDirectoryNode();

  components.forEach((component) => {
    const segments = component.directory
      .split(path.sep)
      .filter(Boolean);
    let current = root;

    segments.forEach((segment) => {
      if (!current.children.has(segment)) {
        current.children.set(segment, createDirectoryNode(segment));
      }
      current = current.children.get(segment);
    });

    current.components.push(component);
  });

  return root;
};

const fileDescription = (filename) => {
  if (filename.endsWith('.spec.ts')) return 'Prueba';
  if (filename.endsWith('.html')) return 'Template';
  if (/\.(scss|css|sass|less)$/.test(filename)) return 'Estilos';
  return 'Componente';
};

const getTreeSymbols = (unicode) => unicode
  ? { branch: 'â”œâ”€â”€ ', last: 'â””â”€â”€ ', line: 'â”‚   ', separator: ' Â· ' }
  : { branch: '|-- ', last: '`-- ', line: '|   ', separator: ' - ' };

const renderComponent = (component, prefix, isLast, colorEnabled, symbols) => {
  const connector = isLast ? symbols.last : symbols.branch;
  const continuation = isLast ? '    ' : symbols.line;
  const metadata = `${component.selector}${symbols.separator}${component.standalone ? 'standalone' : 'NgModule'}`;
  const lines = [
    `${prefix}${connector}${paint(colorEnabled, 'cyan', component.className)} ${paint(colorEnabled, 'dim', `[${metadata}]`)}`,
  ];

  component.files.forEach((file, index) => {
    const fileIsLast = index === component.files.length - 1;
    const fileConnector = fileIsLast ? symbols.last : symbols.branch;
    const filename = path.basename(file);
    lines.push(
      `${prefix}${continuation}${fileConnector}${paint(colorEnabled, 'yellow', fileDescription(filename))}: ${filename}`,
    );
  });

  return lines;
};

const renderDirectory = (node, prefix, colorEnabled, symbols) => {
  const directories = [...node.children.values()].sort((a, b) =>
    a.name.localeCompare(b.name),
  );
  const components = [...node.components].sort((a, b) =>
    a.className.localeCompare(b.className),
  );
  const entries = [
    ...directories.map((directory) => ({ directory })),
    ...components.map((component) => ({ component })),
  ];
  const lines = [];

  entries.forEach((entry, index) => {
    const isLast = index === entries.length - 1;
    const connector = isLast ? symbols.last : symbols.branch;
    const continuation = isLast ? '    ' : symbols.line;

    if (entry.directory) {
      lines.push(
        `${prefix}${connector}${paint(colorEnabled, 'blue', `${entry.directory.name}/`)}`,
      );
      lines.push(
        ...renderDirectory(
          entry.directory,
          `${prefix}${continuation}`,
          colorEnabled,
          symbols,
        ),
      );
    } else {
      lines.push(
        ...renderComponent(entry.component, prefix, isLast, colorEnabled, symbols),
      );
    }
  });

  return lines;
};

const pluralize = (count) => `${count} componente${count === 1 ? '' : 's'}`;

const createReport = (rootPath, components, colorEnabled, unicode) => {
  const title = paint(colorEnabled, 'magenta', 'Arbol de componentes Angular');
  const rootLabel = paint(colorEnabled, 'green', `${path.basename(rootPath)}/`);
  const symbols = getTreeSymbols(unicode);
  const treeLines = renderDirectory(buildTree(components), '', colorEnabled, symbols);

  return [
    '',
    unicode ? `â—† ${title}` : `== ${title} ==`,
    `  ${paint(colorEnabled, 'dim', `${pluralize(components.length)} encontrados`)}`,
    '',
    rootLabel,
    ...treeLines,
    '',
  ].join('\n');
};

const main = async () => {
  const options = parseArguments(process.argv.slice(2));
  const rootPath = path.resolve(options.root);
  const stat = await fs.stat(rootPath).catch(() => null);

  if (!stat?.isDirectory()) {
    throw new Error(`No se encontrÃ³ el directorio: ${rootPath}`);
  }

  const files = await walk(rootPath);
  const typescriptFiles = files.filter(
    (file) => file.endsWith('.ts') && !file.endsWith('.spec.ts') && !file.endsWith('.d.ts'),
  );
  const inspected = await Promise.all(
    typescriptFiles.map((file) => inspectComponent(file, rootPath)),
  );
  const components = inspected.filter(Boolean);
  const terminalReport = createReport(
    rootPath,
    components,
    options.color,
    options.unicode,
  );

  process.stdout.write(terminalReport);

  if (options.output) {
    const outputPath = path.resolve(options.output);
    const plainReport = createReport(rootPath, components, false, options.unicode);
    await fs.mkdir(path.dirname(outputPath), { recursive: true });
    await fs.writeFile(outputPath, plainReport, 'utf8');
    process.stdout.write(`Guardado en ${outputPath}\n`);
  }
};

main().catch((error) => {
  process.stderr.write(`Error: ${error.message}\n`);
  process.exitCode = 1;
});