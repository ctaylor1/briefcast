// Node module hooks for the unit test runner.
//
// They let `node --test` import the app's TypeScript and single-file components
// the way Vite does, without a separate test framework:
// - extensionless relative imports resolve to `.ts`, `.vue` or `index.ts`;
// - `.ts` files are transpiled with the project's TypeScript;
// - `.vue` files are compiled with `vue/compiler-sfc` (template inlined, styles dropped);
// - CSS imports become empty modules and image imports become their URL.
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { basename } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { compileScript, parse, registerTS } from "vue/compiler-sfc";

registerTS(() => ts);

const CANDIDATE_SUFFIXES = [".ts", ".vue", "/index.ts"];
const ASSET_PATTERN = /\.(svg|png|jpe?g|gif|webp|ico)$/;

function isRelative(specifier) {
  return specifier.startsWith("./") || specifier.startsWith("../");
}

export async function resolve(specifier, context, nextResolve) {
  try {
    return await nextResolve(specifier, context);
  } catch (error) {
    const recoverable = error?.code === "ERR_MODULE_NOT_FOUND" || error?.code === "ERR_UNSUPPORTED_DIR_IMPORT";
    if (!recoverable || !isRelative(specifier)) {
      throw error;
    }
    for (const suffix of CANDIDATE_SUFFIXES) {
      try {
        return await nextResolve(specifier + suffix, context);
      } catch {
        // try the next candidate
      }
    }
    throw error;
  }
}

function transpileTypeScript(source, fileName) {
  return ts.transpileModule(source, {
    fileName,
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
      isolatedModules: true,
      sourceMap: false,
      inlineSourceMap: true,
      inlineSources: true,
    },
  }).outputText;
}

function compileVue(source, fileName) {
  const { descriptor, errors } = parse(source, { filename: fileName });
  if (errors.length > 0) {
    throw new Error(`Failed to parse ${fileName}: ${errors.map((e) => e.message).join("; ")}`);
  }
  const id = createHash("sha256").update(fileName).digest("hex").slice(0, 8);
  const script = compileScript(descriptor, {
    id,
    inlineTemplate: true,
    isProd: false,
    genDefaultAs: "_sfc_main",
    fs: {
      fileExists: ts.sys.fileExists,
      readFile: ts.sys.readFile,
    },
  });
  const code = `${script.content}\n_sfc_main.__file = ${JSON.stringify(fileName)};\nexport default _sfc_main;\n`;
  const lang = descriptor.scriptSetup?.lang ?? descriptor.script?.lang;
  return lang === "ts" ? transpileTypeScript(code, `${fileName}.ts`) : code;
}

export async function load(url, context, nextLoad) {
  if (!url.startsWith("file:") || url.includes("/node_modules/")) {
    return nextLoad(url, context);
  }
  const fileName = fileURLToPath(url);
  if (url.endsWith(".css")) {
    return { format: "module", source: "export default '';", shortCircuit: true };
  }
  if (ASSET_PATTERN.test(url)) {
    const source = `export default ${JSON.stringify(`/app/assets/${basename(fileName)}`)};`;
    return { format: "module", source, shortCircuit: true };
  }
  if (url.endsWith(".vue")) {
    const source = await readFile(fileName, "utf8");
    return { format: "module", source: compileVue(source, fileName), shortCircuit: true };
  }
  if (url.endsWith(".ts")) {
    const source = await readFile(fileName, "utf8");
    return { format: "module", source: transpileTypeScript(source, fileName), shortCircuit: true };
  }
  return nextLoad(url, context);
}
