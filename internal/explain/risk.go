package explain

import "strings"

// HighRiskMissedDose — что делать при пропуске для лекарств высокого риска.
// Ставится кодом, ответ модели для этого поля игнорируется.
const HighRiskMissedDose = "Не принимайте пропущенную дозу самостоятельно — позвоните врачу."

// Определяется только по МНН: кода АТХ в справочнике лекарств (medicines) пока нет,
// поэтому на практике atc = "" и список МНН должен покрывать все группы сам.
//
// highRiskMNN — части МНН (рус. и лат., в нижнем регистре) групп высокого риска:
// антикоагулянты, инсулины, противоэпилептические, гормональные контрацептивы,
// метотрексат и противоопухолевые, иммунодепрессанты, противопаркинсонические.
var highRiskMNN = []string{
	// антикоагулянты
	"варфарин", "warfarin", "аценокумарол", "acenocoumarol", "фениндион", "phenindione",
	"ривароксабан", "rivaroxaban", "апиксабан", "apixaban", "дабигатран", "dabigatran",
	// инсулины
	"инсулин", "insulin",
	// противоэпилептические
	"карбамазепин", "carbamazepine", "окскарбазепин", "oxcarbazepine",
	"вальпро", "valpro", "ламотриджин", "lamotrigine", "леветирацетам", "levetiracetam",
	"фенитоин", "phenytoin", "топирамат", "topiramate",
	"габапентин", "gabapentin", "прегабалин", "pregabalin", "клоназепам", "clonazepam",
	"лакосамид", "lacosamide", "этосуксимид", "ethosuximide", "примидон", "primidone",
	"зонисамид", "zonisamide", "перампанел", "perampanel", "бриварацетам", "brivaracetam",
	// фенобарбитал намеренно нет: он входит в Корвалол
	// противоопухолевые и метотрексат
	"метотрексат", "methotrexate", "капецитабин", "capecitabine",
	"гидроксикарбамид", "hydroxycarbamide", "хлорамбуцил", "chlorambucil", "мелфалан", "melphalan",
	"темозоломид", "temozolomide",
	"тиниб", "tinib", // ингибиторы киназ: иматиниб, тофацитиниб, барицитиниб, упадацитиниб…
	// иммунодепрессанты
	"такролимус", "tacrolimus", "циклоспорин", "ciclosporin", "cyclosporin",
	"микофенол", "mycophenol", "азатиоприн", "azathioprine", "сиролимус", "sirolimus", "эверолимус", "everolimus",
	"лефлуномид", "leflunomide", "терифлуномид", "teriflunomide", "финголимод", "fingolimod",
	// противопаркинсонические
	"леводопа", "levodopa", "прамипексол", "pramipexole", "ропинирол", "ropinirole",
	"амантадин", "amantadine", "разагилин", "rasagiline", "селегилин", "selegiline",
	"пирибедил", "piribedil", "ротиготин", "rotigotine", "энтакапон", "entacapone",
	"бромокриптин", "bromocriptine",
	// гормональные контрацептивы
	"этинилэстрадиол", "ethinylestradiol", "левоноргестрел", "levonorgestrel",
	"дезогестрел", "desogestrel", "гестоден", "gestodene", "дроспиренон", "drospirenone",
	"диеногест", "dienogest", "номегэстрол", "номегестрол", "nomegestrol", "хлормадинон", "chlormadinone",
	"норэтистерон", "norethisterone", "эстрадиола валерат", "estradiol valerate",
}

// highRiskATC — префиксы кодов АТХ тех же групп.
var highRiskATC = []string{"B01AA", "A10A", "N03A", "L01", "L04", "N04B", "G03A"}

// IsHighRisk: лекарство из группы высокого риска по МНН или коду АТХ.
func IsHighRisk(mnn, atc string) bool {
	m := strings.ToLower(mnn)
	if m != "" {
		for _, s := range highRiskMNN {
			if strings.Contains(m, s) {
				return true
			}
		}
	}
	a := strings.ToUpper(strings.TrimSpace(atc))
	if a != "" {
		for _, p := range highRiskATC {
			if strings.HasPrefix(a, p) {
				return true
			}
		}
	}
	return false
}
