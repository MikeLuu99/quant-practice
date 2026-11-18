import goWorker from './build/worker.mjs';
import revision from './build/revision.mjs';
import {createGateway} from './router.mjs';
export {GameSession} from './build/worker.mjs';
export default createGateway({fetchGo:goWorker.fetch,revision});
