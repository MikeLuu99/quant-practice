import { copyFile, mkdir } from 'node:fs/promises';
const output = new URL('../public/specifications/', import.meta.url);
await mkdir(output, { recursive: true });
for (const name of ['market_making_games_specification.md', 'tradermath_assessments_specification_and_overlap.md', 'tradermath_trainers_specification_and_overlap.md']) {
  await copyFile(new URL(`../../${name}`, import.meta.url), new URL(name, output));
}
