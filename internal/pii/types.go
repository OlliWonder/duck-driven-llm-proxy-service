// Пакет pii определяет канонический набор типов персональных данных и
// общий контракт, используемый во всём сервисе.
//
// Приведённые ниже 17 названий типов — фиксированный согласованный словарь.
// Детекторы (участник 2) возвращают фрагменты с этими типами; слои
// маскирования и политик (участник 1) их потребляют. Не переименовывайте
// эти константы без обновления всех потребителей.
package pii

// Type — канонический идентификатор типа персональных данных.
type Type string

// 17 обязательных типов персональных данных.
const (
	TypeFullName          Type = "full_name"           // ФИО
	TypeBirthDate         Type = "birth_date"          // Дата рождения
	TypeBirthPlace        Type = "birth_place"         // Место рождения
	TypePassportSeries    Type = "passport_series"     // Серия и номер паспорта
	TypeCitizenship       Type = "citizenship"         // Гражданство
	TypePassportIssuer    Type = "passport_issuer"     // Орган, выдавший паспорт
	TypePassportDeptCode  Type = "passport_dept_code"  // Код подразделения
	TypePassportIssueDate Type = "passport_issue_date" // Дата выдачи паспорта
	TypeDrivingLicense    Type = "driving_license"     // Серия и номер водительского удостоверения
	TypeAddress           Type = "address"             // Адрес (страна, индекс, город, улица, дом, квартира)
	TypeEmail             Type = "email"               // Email
	TypePhone             Type = "phone"               // Номер телефона
	TypeINN               Type = "inn"                 // ИНН
	TypeCVV               Type = "cvv"                 // CVV-код
	TypePIN               Type = "pin"                 // Пин-код карты
	TypeCardHolder        Type = "card_holder"         // Имя держателя карты
	TypeCardNumber        Type = "card_number"         // Номер платёжной банковской карты
)

// AllTypes перечисляет все поддерживаемые типы в стабильном порядке.
var AllTypes = []Type{
	TypeFullName,
	TypeBirthDate,
	TypeBirthPlace,
	TypePassportSeries,
	TypeCitizenship,
	TypePassportIssuer,
	TypePassportDeptCode,
	TypePassportIssueDate,
	TypeDrivingLicense,
	TypeAddress,
	TypeEmail,
	TypePhone,
	TypeINN,
	TypeCVV,
	TypePIN,
	TypeCardHolder,
	TypeCardNumber,
}

// Valid сообщает, является ли t одним из канонических типов.
func (t Type) Valid() bool {
	for _, known := range AllTypes {
		if t == known {
			return true
		}
	}
	return false
}