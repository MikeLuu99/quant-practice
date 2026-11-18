import { describe, expect, test } from "bun:test";
import profiles from "../../../backend/internal/games/assessment_profiles.json";
import { cubeSVG, renderAssessmentAsset, renderAssessmentInputs, responseSummary, type AssessmentQuestion } from "./assessment-ui";
const question:AssessmentQuestion={question_id:'q001',family:'probability',prompt:'P(event)?',response_type:'numeric'};

describe('assessment catalogue',()=>{
  test('25 profiles cover 11 companies and every overlap links to an existing game',()=>{
    expect(profiles).toHaveLength(25);expect(new Set(profiles.map(p=>p.company_id)).size).toBe(11);
    expect(new Set(profiles.map(p=>p.id)).size).toBe(25);
    const slugs=new Set(['math','fruit','prob','nextcard','cards','fermi','makeme','etf','marble']);
    for(const p of profiles){expect(p.question_count).toBeGreaterThan(0);expect(p.families.length).toBeGreaterThan(0);for(const slug of p.overlap)expect(slugs.has(slug)).toBe(true);}
  });
  test('versions retain published format differences and explicitly label unknowns',()=>{
    expect(profiles.find(p=>p.id==='maven-numerical-classic')?.response_mode).toBe('single_choice');
    expect(profiles.find(p=>p.id==='maven-numerical-new')?.response_mode).toBe('numeric');
    expect(profiles.find(p=>p.id==='maven-sequences-new')?.families).toContain('anomaly');
    expect(profiles.find(p=>p.id==='drw-assessment')?.navigation).toBe('revisable');
    expect(profiles.find(p=>p.id==='da-vinci-career')?.content_origin).toBe('original demo');
    expect(profiles.find(p=>p.id==='akuna-grid')?.notes).toContain('does not publish');
  });
});
describe('assessment question controls',()=>{
  test('numeric drafts and reasoning are escaped when restored',()=>{
    const html=renderAssessmentInputs(question,{answer:'"><script>alert(1)</script>'});
    expect(html).not.toContain('<script>');expect(html).toContain('&lt;script&gt;');
    const interval=renderAssessmentInputs({...question,response_type:'interval'},{low:'1',high:'2',reasoning:'</textarea><script>x</script>'});
    expect(interval).not.toContain('</textarea><script>');expect(interval).toContain('&lt;/textarea&gt;');
  });
  test('choice IDs survive saved response restoration',()=>{
    const html=renderAssessmentInputs({...question,response_type:'single_choice',choices:[{id:'a',text:'1/3'},{id:'b',text:'2/3'}]},{answer:'b'});
    expect(responseSummary({answer:'b'},{...question,response_type:'single_choice',choices:[{id:'b',text:'2/3'}]})).toBe('B: 2/3');
    expect(html).toContain('value="b" checked');expect(html).toContain('Choose one answer');
  });
  test('sequence anomaly and next-term tasks render different prompts',()=>{
    const assets={kind:'sequence',tokens:['3','7','11','16','19'],task:'anomaly'};
    expect(renderAssessmentAsset({...question,assets})).not.toContain('sequence-unknown');
    expect(renderAssessmentAsset({...question,assets:{...assets,task:'sequence'}})).toContain('sequence-unknown');
  });
  test('cube net and views carry a reproducible accessible representation',()=>{
    const asset={kind:'cube',net:[{x:1,y:0,label:'A'},{x:1,y:1,label:'B'}],candidates:[]};
    const html=renderAssessmentAsset({...question,assets:asset});expect(html).toContain('column 2, row 1');expect(html).toContain('grid-column:2;grid-row:1');
    expect(cubeSVG({top:'1',front:'2',right:'3'})).toContain('top 1, front 2, right 3');
  });
  test('grid rules, all rows, probabilities and bet costs are candidate-visible',()=>{
    const assets={kind:'grid',rows:Array.from({length:13},(_,i)=>[String.fromCharCode(65+i),String.fromCharCode(78+i)]),probabilities:Array(13).fill('1/13'),bets:[{id:'a',components:[{letter:'A',sign:1}],cost:'1/101'}],rules:'Original signed payoff model'};
    const html=renderAssessmentAsset({...question,assets});expect((html.match(/class="letter-row"/g)||[]).length).toBe(13);expect(html).toContain('1/101');expect(html).toContain('1/13');expect(html).toContain('Original signed payoff model');
  });
  test('blank and interval responses have distinct review text',()=>{
    expect(responseSummary({})).toBe('Unanswered');expect(responseSummary({low:'80',high:'120'})).toBe('[80, 120]');
  });
});
