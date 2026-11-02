package explain

import "strings"

// HighRiskMissedDose — что делать при пропуске для лекарств высокого риска.
// Ставится кодом, ответ модели для этого поля игнорируется.
const HighRiskMissedDose = "Не принимайте пропущенную дозу самостоятельно — позвоните врачу."

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
	// противоопухолевые и метотрексат
	"метотрексат", "methotrexate", "капецитабин", "capecitabine", "иматиниб", "imatinib",
	// иммунодепрессанты
	"такролимус", "tacrolimus", "циклоспорин", "ciclosporin", "cyclosporin",
	"микофенол", "mycophenol", "азатиоприн", "azathioprine", "сиролимус", "sirolimus", "эверолимус", "everolimus",
	// противопаркинсонические
	"леводопа", "levodopa", "прамипексол", "pramipexole", "ропинирол", "ropinirole",
	// гормональные контрацептивы
	"этинилэстрадиол", "ethinylestradiol", "левоноргестрел", "levonorgestrel",
	"дезогестрел", "desogestrel", "гестоден", "gestodene", "дроспиренон", "drospirenone",
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
