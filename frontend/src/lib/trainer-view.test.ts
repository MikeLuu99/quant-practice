import {describe,expect,test} from 'bun:test';
import profiles from '../../../backend/internal/games/trainer_profiles.json';
import {trainerGuides} from './trainer-guides';
import {renderTrainerBoard,simulatorValue,cubeFace} from './trainer-view';
describe('trainer public UI contract',()=>{
 test('every catalogue trainer has complete inputs/actions/outputs/scoring',()=>{expect(profiles.length).toBe(22);for(const p of profiles){const guide=trainerGuides[p.id];for(const key of ['input','action','output','score','tip'] as const)expect(guide[key].length).toBeGreaterThan(20)}});
 test('all six cube IDs map to concrete face textures',()=>{for(const label of ['1','2','3','4','5','6']){expect(cubeFace(label,'colors')).not.toContain('undefined');expect(cubeFace(label,'dice')).toMatch(/[⚀⚁⚂⚃⚄⚅]/)}});
 test('memory response never renders past encoding cells or digits',()=>{const state={phase:'RESPONSE',kind:'grid-recall',scene:{span:3},move_state:{},config:{mode:'standard'}};const html=renderTrainerBoard(state);expect(html.match(/data-cell=/g)?.length).toBe(16);expect(html).not.toContain('class="memory-cell blue');expect(html).not.toContain('class="memory-cell red');const pin=renderTrainerBoard({...state,kind:'pincode',scene:{length:5,mode:'repeat'}});expect(pin).toContain('maxlength="5"');expect(pin).not.toContain('trainer-digit"')});
 test('simulators preserve units and explicit pricing models',()=>{expect(simulatorValue('etf',{a:2,p:40,b:1,q:30,d:1})).toBe('Basket NAV: 110');expect(simulatorValue('options',{s:100,v:.2,t:.25})).toContain(': 8');expect(simulatorValue('futures',{s:100,c:.04,t:.5})).toContain(': 102');expect(simulatorValue('prediction',{p:.4,l:3})).toContain('0.66666667');expect(simulatorValue('etf',{a:2,p:40,b:1,q:30,d:0})).toContain('finite')});
 test('lessons and questions escape supplied content',()=>{const html=renderTrainerBoard({phase:'RESPONSE',kind:'math',config:{mode:'practice'},scene:{},move_state:{},question:{prompt:'<script>bad</script>',response_type:'numeric',question_id:'q1'}});expect(html).toContain('&lt;script&gt;');expect(html).not.toContain('<script>')});
});
