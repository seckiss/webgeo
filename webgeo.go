package webgeo

import (
	"encoding/csv"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	geoip2 "github.com/oschwald/geoip2-golang"
	"golang.org/x/text/language"
)

var country2LangMap = mustBuildCountry2LangMap()
var geoLangsCache = make(map[string][]string)
var geoLangsCacheMutex = sync.RWMutex{}

type GeoRecord struct {
	Ip      string `json:"ip"`
	Cc      string `json:"cc"`
	Country string `json:"country"`
	City    string `json:"city"`
}

func CalcCountryAndLangs(r *http.Request) (string, []string) {
	ipS, _, _ := net.SplitHostPort(r.RemoteAddr)

	var blangs = browserLangs(r)
	glangs := geoLangs(ipS)
	country := glangs[0]
	glangs = glangs[1:]
	//fmt.Printf("blangs=%+v, glangs=%+v\n", blangs, glangs)
	// get unique langs
	var langMap = make(map[string]string)
	for _, b := range blangs {
		langMap[b] = ""
	}
	for _, g := range glangs {
		langMap[g] = ""
	}
	// eliminate generic language codes when country specific langs are present
	var countrySpecific = make(map[string]string)
	for k, _ := range langMap {
		if strings.Contains(k, "-") {
			countrySpecific[k] = ""
		}
	}
	for k, _ := range countrySpecific {
		delete(langMap, strings.Split(k, "-")[0])
	}
	var langs = []string{}
	for k, _ := range langMap {
		langs = append(langs, k)
	}

	//fmt.Printf("\n\ncalcLangs: %v\n\n", langs)
	return country, langs
}

// Parse http request heeader "Accept-Language" to get the list of lang-region codes
func browserLangs(r *http.Request) []string {
	var langs = []string{}
	tags, _, err := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if err == nil {
		for i := 0; i < len(tags); i++ {
			langs = append(langs, tags[i].String())
		}
	}
	return langs
}

// returns list:
// - 0th element is country code (ZZ if unidentified)
// - alternative 1st and 2nd element are suggested languages for the region
func geoLangs(ipS string) []string {
	geoLangsCacheMutex.RLock()
	if l, pres := geoLangsCache[ipS]; pres {
		geoLangsCacheMutex.RUnlock()
		return l
	}
	geoLangsCacheMutex.RUnlock()

	ip := net.ParseIP(ipS)
	geo, err := geolocate(ip)
	var langs = []string{}
	if len(geo.Cc) == 2 {
		langs = append(langs, strings.ToUpper(geo.Cc))
		if err == nil {
			// comma separated languages
			if csl, pres := country2LangMap[strings.ToUpper(geo.Cc)]; pres {
				tags, _, err := language.ParseAcceptLanguage(csl)
				if err == nil {
					for i := 0; i < len(tags); i++ {
						langs = append(langs, tags[i].String())
					}
				}
			}
		}
	} else {
		langs = append(langs, "ZZ")
	}
	geoLangsCacheMutex.Lock()
	geoLangsCache[ipS] = langs
	geoLangsCacheMutex.Unlock()
	//fmt.Printf("\n\ngeoLangs: %v\n\n", langs)
	return langs
}

func geolocate(ip net.IP) (*GeoRecord, error) {
	mmdbfile := "GeoLite2-City.mmdb"

	if _, err := os.Stat(mmdbfile); err != nil {
		log.Printf("%s does not exist. Checking for gz...", mmdbfile)
		if _, err := os.Stat(mmdbfile + ".gz"); err != nil {
			log.Printf("%s.gz does not exist. Downloading...", mmdbfile)
			exec.Command("wget", "-N", "http://geolite.maxmind.com/download/geoip/database/GeoLite2-City.mmdb.gz").Output()
		}
		if _, err := os.Stat(mmdbfile + ".gz"); err != nil {
			return nil, fmt.Errorf("Could not download %s.gz", mmdbfile)
		}
		log.Printf("Unzip %s.gz", mmdbfile)
		exec.Command("gunzip", mmdbfile+".gz").Output()
		if _, err := os.Stat(mmdbfile); err != nil {
			return nil, fmt.Errorf("Could not unzip %s.gz", mmdbfile)
		}
	}

	db, err := geoip2.Open(mmdbfile)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	record, err := db.City(ip)
	if err != nil {
		return nil, err
	}
	cc := record.Country.IsoCode
	country := record.Country.Names["en"]
	city := record.City.Names["en"]
	return &GeoRecord{ip.String(), cc, country, city}, nil
}

func readCountryInfoTable() ([][]string, error) {
	/*
		f, err := os.Open("countryInfoTrimmed.txt")
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r := csv.NewReader(bufio.NewReader(f))
	*/
	r := csv.NewReader(strings.NewReader(countryInfoTable))
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	return records, nil
}

func buildCountry2LangMap() (map[string]string, error) {
	records, err := readCountryInfoTable()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for _, r := range records {
		langs := strings.Split(r[6], ",")
		//take maximum 2 languages
		if len(langs) > 1 {
			m[r[0]] = langs[0] + "," + langs[1]
		} else {
			m[r[0]] = langs[0]
		}
	}
	//fmt.Printf("%+v\n", m)
	return m, nil
}

func mustBuildCountry2LangMap() map[string]string {
	m, err := buildCountry2LangMap()
	if err != nil {
		panic(err)
	}
	return m
}

var countryInfoTable = `
AD,Andorra,EU,.ad,EUR,Euro,ca
AE,United Arab Emirates,AS,.ae,AED,Dirham,"ar-AE,fa,en,hi,ur"
AF,Afghanistan,AS,.af,AFN,Afghani,"fa-AF,ps,uz-AF,tk"
AG,Antigua and Barbuda,NA,.ag,XCD,Dollar,en-AG
AI,Anguilla,NA,.ai,XCD,Dollar,en-AI
AL,Albania,EU,.al,ALL,Lek,"sq,el"
AM,Armenia,AS,.am,AMD,Dram,hy
AO,Angola,AF,.ao,AOA,Kwanza,pt-AO
AR,Argentina,SA,.ar,ARS,Peso,"es-AR,en,it,de,fr,gn"
AS,American Samoa,OC,.as,USD,Dollar,"en-AS,sm,to"
AT,Austria,EU,.at,EUR,Euro,"de-AT,hr,hu,sl"
AU,Australia,OC,.au,AUD,Dollar,en-AU
AW,Aruba,NA,.aw,AWG,Guilder,"nl-AW,es,en"
AX,Aland Islands,EU,.ax,EUR,Euro,sv-AX
AZ,Azerbaijan,AS,.az,AZN,Manat,"az,ru,hy"
BA,Bosnia and Herzegovina,EU,.ba,BAM,Marka,"bs,hr-BA,sr-BA"
BB,Barbados,NA,.bb,BBD,Dollar,en-BB
BD,Bangladesh,AS,.bd,BDT,Taka,"bn-BD,en"
BE,Belgium,EU,.be,EUR,Euro,"nl-BE,fr-BE,de-BE"
BF,Burkina Faso,AF,.bf,XOF,Franc,fr-BF
BG,Bulgaria,EU,.bg,BGN,Lev,"bg,tr-BG,rom"
BH,Bahrain,AS,.bh,BHD,Dinar,"ar-BH,en,fa,ur"
BI,Burundi,AF,.bi,BIF,Franc,"fr-BI,rn"
BJ,Benin,AF,.bj,XOF,Franc,fr-BJ
BL,Saint Barthelemy,NA,.gp,EUR,Euro,fr
BM,Bermuda,NA,.bm,BMD,Dollar,"en-BM,pt"
BN,Brunei,AS,.bn,BND,Dollar,"ms-BN,en-BN"
BO,Bolivia,SA,.bo,BOB,Boliviano,"es-BO,qu,ay"
BQ,"Bonaire, Saint Eustatius and Saba ",NA,.bq,USD,Dollar,"nl,pap,en"
BR,Brazil,SA,.br,BRL,Real,"pt-BR,es,en,fr"
BS,Bahamas,NA,.bs,BSD,Dollar,en-BS
BT,Bhutan,AS,.bt,BTN,Ngultrum,dz
BW,Botswana,AF,.bw,BWP,Pula,"en-BW,tn-BW"
BY,Belarus,EU,.by,BYR,Ruble,"be,ru"
BZ,Belize,NA,.bz,BZD,Dollar,"en-BZ,es"
CA,Canada,NA,.ca,CAD,Dollar,"en-CA,fr-CA,iu"
CC,Cocos Islands,AS,.cc,AUD,Dollar,"ms-CC,en"
CD,Democratic Republic of the Congo,AF,.cd,CDF,Franc,"fr-CD,ln,kg"
CF,Central African Republic,AF,.cf,XAF,Franc,"fr-CF,sg,ln,kg"
CG,Republic of the Congo,AF,.cg,XAF,Franc,"fr-CG,kg,ln-CG"
CH,Switzerland,EU,.ch,CHF,Franc,"de-CH,fr-CH,it-CH,rm"
CI,Ivory Coast,AF,.ci,XOF,Franc,fr-CI
CK,Cook Islands,OC,.ck,NZD,Dollar,"en-CK,mi"
CL,Chile,SA,.cl,CLP,Peso,es-CL
CM,Cameroon,AF,.cm,XAF,Franc,"en-CM,fr-CM"
CN,China,AS,.cn,CNY,Yuan Renminbi,"zh-CN,yue,wuu,dta,ug,za"
CO,Colombia,SA,.co,COP,Peso,es-CO
CR,Costa Rica,NA,.cr,CRC,Colon,"es-CR,en"
CU,Cuba,NA,.cu,CUP,Peso,es-CU
CV,Cape Verde,AF,.cv,CVE,Escudo,pt-CV
CW,Curacao,NA,.cw,ANG,Guilder,"nl,pap"
CX,Christmas Island,AS,.cx,AUD,Dollar,"en,zh,ms-CC"
CY,Cyprus,EU,.cy,EUR,Euro,"el-CY,tr-CY,en"
CZ,Czechia,EU,.cz,CZK,Koruna,"cs,sk"
DE,Germany,EU,.de,EUR,Euro,de
DJ,Djibouti,AF,.dj,DJF,Franc,"fr-DJ,ar,so-DJ,aa"
DK,Denmark,EU,.dk,DKK,Krone,"da-DK,en,fo,de-DK"
DM,Dominica,NA,.dm,XCD,Dollar,en-DM
DO,Dominican Republic,NA,.do,DOP,Peso,es-DO
DZ,Algeria,AF,.dz,DZD,Dinar,ar-DZ
EC,Ecuador,SA,.ec,USD,Dollar,es-EC
EE,Estonia,EU,.ee,EUR,Euro,"et,ru"
EG,Egypt,AF,.eg,EGP,Pound,"ar-EG,en,fr"
EH,Western Sahara,AF,.eh,MAD,Dirham,"ar,mey"
ER,Eritrea,AF,.er,ERN,Nakfa,"aa-ER,ar,tig,kun,ti-ER"
ES,Spain,EU,.es,EUR,Euro,"es-ES,ca,gl,eu,oc"
ET,Ethiopia,AF,.et,ETB,Birr,"am,en-ET,om-ET,ti-ET,so-ET,sid"
FI,Finland,EU,.fi,EUR,Euro,"fi-FI,sv-FI,smn"
FJ,Fiji,OC,.fj,FJD,Dollar,"en-FJ,fj"
FK,Falkland Islands,SA,.fk,FKP,Pound,en-FK
FM,Micronesia,OC,.fm,USD,Dollar,"en-FM,chk,pon,yap,kos,uli,woe,nkr,kpg"
FO,Faroe Islands,EU,.fo,DKK,Krone,"fo,da-FO"
FR,France,EU,.fr,EUR,Euro,"fr-FR,frp,br,co,ca,eu,oc"
GA,Gabon,AF,.ga,XAF,Franc,fr-GA
GB,United Kingdom,EU,.uk,GBP,Pound,"en-GB,cy-GB,gd"
GD,Grenada,NA,.gd,XCD,Dollar,en-GD
GE,Georgia,AS,.ge,GEL,Lari,"ka,ru,hy,az"
GF,French Guiana,SA,.gf,EUR,Euro,fr-GF
GG,Guernsey,EU,.gg,GBP,Pound,"en,fr"
GH,Ghana,AF,.gh,GHS,Cedi,"en-GH,ak,ee,tw"
GI,Gibraltar,EU,.gi,GIP,Pound,"en-GI,es,it,pt"
GL,Greenland,NA,.gl,DKK,Krone,"kl,da-GL,en"
GM,Gambia,AF,.gm,GMD,Dalasi,"en-GM,mnk,wof,wo,ff"
GN,Guinea,AF,.gn,GNF,Franc,fr-GN
GP,Guadeloupe,NA,.gp,EUR,Euro,fr-GP
GQ,Equatorial Guinea,AF,.gq,XAF,Franc,"es-GQ,fr"
GR,Greece,EU,.gr,EUR,Euro,"el-GR,en,fr"
GS,South Georgia and the South Sandwich Islands,AN,.gs,GBP,Pound,en
GT,Guatemala,NA,.gt,GTQ,Quetzal,es-GT
GU,Guam,OC,.gu,USD,Dollar,"en-GU,ch-GU"
GW,Guinea-Bissau,AF,.gw,XOF,Franc,"pt-GW,pov"
GY,Guyana,SA,.gy,GYD,Dollar,en-GY
HK,Hong Kong,AS,.hk,HKD,Dollar,"zh-HK,yue,zh,en"
HN,Honduras,NA,.hn,HNL,Lempira,es-HN
HR,Croatia,EU,.hr,HRK,Kuna,"hr-HR,sr"
HT,Haiti,NA,.ht,HTG,Gourde,"ht,fr-HT"
HU,Hungary,EU,.hu,HUF,Forint,hu-HU
ID,Indonesia,AS,.id,IDR,Rupiah,"id,en,nl,jv"
IE,Ireland,EU,.ie,EUR,Euro,"en-IE,ga-IE"
IL,Israel,AS,.il,ILS,Shekel,"he,ar-IL,en-IL,"
IM,Isle of Man,EU,.im,GBP,Pound,"en,gv"
IN,India,AS,.in,INR,Rupee,"en-IN,hi,bn,te,mr,ta,ur,gu,kn,ml,or,pa,as,bh,sat,ks,ne,sd,kok,doi,mni,sit,sa,fr,lus,inc"
IO,British Indian Ocean Territory,AS,.io,USD,Dollar,en-IO
IQ,Iraq,AS,.iq,IQD,Dinar,"ar-IQ,ku,hy"
IR,Iran,AS,.ir,IRR,Rial,"fa-IR,ku"
IS,Iceland,EU,.is,ISK,Krona,"is,en,de,da,sv,no"
IT,Italy,EU,.it,EUR,Euro,"it-IT,de-IT,fr-IT,sc,ca,co,sl"
JE,Jersey,EU,.je,GBP,Pound,"en,pt"
JM,Jamaica,NA,.jm,JMD,Dollar,en-JM
JO,Jordan,AS,.jo,JOD,Dinar,"ar-JO,en"
JP,Japan,AS,.jp,JPY,Yen,ja
KE,Kenya,AF,.ke,KES,Shilling,"en-KE,sw-KE"
KG,Kyrgyzstan,AS,.kg,KGS,Som,"ky,ru,uz"
KH,Cambodia,AS,.kh,KHR,Riels,"km,fr,en"
KI,Kiribati,OC,.ki,AUD,Dollar,"en-KI,gil"
KM,Comoros,AF,.km,KMF,Franc,"ar,fr-KM"
KN,Saint Kitts and Nevis,NA,.kn,XCD,Dollar,en-KN
KP,North Korea,AS,.kp,KPW,Won,ko-KP
KR,South Korea,AS,.kr,KRW,Won,"ko-KR,en"
XK,Kosovo,EU,,EUR,Euro,"sq,sr"
KW,Kuwait,AS,.kw,KWD,Dinar,"ar-KW,en"
KY,Cayman Islands,NA,.ky,KYD,Dollar,en-KY
KZ,Kazakhstan,AS,.kz,KZT,Tenge,"kk,ru"
LA,Laos,AS,.la,LAK,Kip,"lo,fr,en"
LB,Lebanon,AS,.lb,LBP,Pound,"ar-LB,fr-LB,en,hy"
LC,Saint Lucia,NA,.lc,XCD,Dollar,en-LC
LI,Liechtenstein,EU,.li,CHF,Franc,de-LI
LK,Sri Lanka,AS,.lk,LKR,Rupee,"si,ta,en"
LR,Liberia,AF,.lr,LRD,Dollar,en-LR
LS,Lesotho,AF,.ls,LSL,Loti,"en-LS,st,zu,xh"
LT,Lithuania,EU,.lt,EUR,Euro,"lt,ru,pl"
LU,Luxembourg,EU,.lu,EUR,Euro,"lb,de-LU,fr-LU"
LV,Latvia,EU,.lv,EUR,Euro,"lv,ru,lt"
LY,Libya,AF,.ly,LYD,Dinar,"ar-LY,it,en"
MA,Morocco,AF,.ma,MAD,Dirham,"ar-MA,ber,fr"
MC,Monaco,EU,.mc,EUR,Euro,"fr-MC,en,it"
MD,Moldova,EU,.md,MDL,Leu,"ro,ru,gag,tr"
ME,Montenegro,EU,.me,EUR,Euro,"sr,hu,bs,sq,hr,rom"
MF,Saint Martin,NA,.gp,EUR,Euro,fr
MG,Madagascar,AF,.mg,MGA,Ariary,"fr-MG,mg"
MH,Marshall Islands,OC,.mh,USD,Dollar,"mh,en-MH"
MK,Macedonia,EU,.mk,MKD,Denar,"mk,sq,tr,rmm,sr"
ML,Mali,AF,.ml,XOF,Franc,"fr-ML,bm"
MM,Myanmar,AS,.mm,MMK,Kyat,my
MN,Mongolia,AS,.mn,MNT,Tugrik,"mn,ru"
MO,Macao,AS,.mo,MOP,Pataca,"zh,zh-MO,pt"
MP,Northern Mariana Islands,OC,.mp,USD,Dollar,"fil,tl,zh,ch-MP,en-MP"
MQ,Martinique,NA,.mq,EUR,Euro,fr-MQ
MR,Mauritania,AF,.mr,MRO,Ouguiya,"ar-MR,fuc,snk,fr,mey,wo"
MS,Montserrat,NA,.ms,XCD,Dollar,en-MS
MT,Malta,EU,.mt,EUR,Euro,"mt,en-MT"
MU,Mauritius,AF,.mu,MUR,Rupee,"en-MU,bho,fr"
MV,Maldives,AS,.mv,MVR,Rufiyaa,"dv,en"
MW,Malawi,AF,.mw,MWK,Kwacha,"ny,yao,tum,swk"
MX,Mexico,NA,.mx,MXN,Peso,es-MX
MY,Malaysia,AS,.my,MYR,Ringgit,"ms-MY,en,zh,ta,te,ml,pa,th"
MZ,Mozambique,AF,.mz,MZN,Metical,"pt-MZ,vmw"
NA,Namibia,AF,.na,NAD,Dollar,"en-NA,af,de,hz,naq"
NC,New Caledonia,OC,.nc,XPF,Franc,fr-NC
NE,Niger,AF,.ne,XOF,Franc,"fr-NE,ha,kr,dje"
NF,Norfolk Island,OC,.nf,AUD,Dollar,en-NF
NG,Nigeria,AF,.ng,NGN,Naira,"en-NG,ha,yo,ig,ff"
NI,Nicaragua,NA,.ni,NIO,Cordoba,"es-NI,en"
NL,Netherlands,EU,.nl,EUR,Euro,"nl-NL,fy-NL"
NO,Norway,EU,.no,NOK,Krone,"no,nb,nn,se,fi"
NP,Nepal,AS,.np,NPR,Rupee,"ne,en"
NR,Nauru,OC,.nr,AUD,Dollar,"na,en-NR"
NU,Niue,OC,.nu,NZD,Dollar,"niu,en-NU"
NZ,New Zealand,OC,.nz,NZD,Dollar,"en-NZ,mi"
OM,Oman,AS,.om,OMR,Rial,"ar-OM,en,bal,ur"
PA,Panama,NA,.pa,PAB,Balboa,"es-PA,en"
PE,Peru,SA,.pe,PEN,Sol,"es-PE,qu,ay"
PF,French Polynesia,OC,.pf,XPF,Franc,"fr-PF,ty"
PG,Papua New Guinea,OC,.pg,PGK,Kina,"en-PG,ho,meu,tpi"
PH,Philippines,AS,.ph,PHP,Peso,"tl,en-PH,fil"
PK,Pakistan,AS,.pk,PKR,Rupee,"ur-PK,en-PK,pa,sd,ps,brh"
PL,Poland,EU,.pl,PLN,Zloty,pl
PM,Saint Pierre and Miquelon,NA,.pm,EUR,Euro,fr-PM
PN,Pitcairn,OC,.pn,NZD,Dollar,en-PN
PR,Puerto Rico,NA,.pr,USD,Dollar,"en-PR,es-PR"
PS,Palestinian Territory,AS,.ps,ILS,Shekel,ar-PS
PT,Portugal,EU,.pt,EUR,Euro,"pt-PT,mwl"
PW,Palau,OC,.pw,USD,Dollar,"pau,sov,en-PW,tox,ja,fil,zh"
PY,Paraguay,SA,.py,PYG,Guarani,"es-PY,gn"
QA,Qatar,AS,.qa,QAR,Rial,"ar-QA,es"
RE,Reunion,AF,.re,EUR,Euro,fr-RE
RO,Romania,EU,.ro,RON,Leu,"ro,hu,rom"
RS,Serbia,EU,.rs,RSD,Dinar,"sr,hu,bs,rom"
RU,Russia,EU,.ru,RUB,Ruble,"ru"
RW,Rwanda,AF,.rw,RWF,Franc,"rw,en-RW,fr-RW,sw"
SA,Saudi Arabia,AS,.sa,SAR,Rial,ar-SA
SB,Solomon Islands,OC,.sb,SBD,Dollar,"en-SB,tpi"
SC,Seychelles,AF,.sc,SCR,Rupee,"en-SC,fr-SC"
SD,Sudan,AF,.sd,SDG,Pound,"ar-SD,en,fia"
SS,South Sudan,AF,,SSP,Pound,en
SE,Sweden,EU,.se,SEK,Krona,"sv-SE,se,sma,fi-SE"
SG,Singapore,AS,.sg,SGD,Dollar,"cmn,en-SG,ms-SG,ta-SG,zh-SG"
SH,Saint Helena,AF,.sh,SHP,Pound,en-SH
SI,Slovenia,EU,.si,EUR,Euro,"sl,sh"
SJ,Svalbard and Jan Mayen,EU,.sj,NOK,Krone,"no,ru"
SK,Slovakia,EU,.sk,EUR,Euro,"sk,hu"
SL,Sierra Leone,AF,.sl,SLL,Leone,"en-SL,men,tem"
SM,San Marino,EU,.sm,EUR,Euro,it-SM
SN,Senegal,AF,.sn,XOF,Franc,"fr-SN,wo,fuc,mnk"
SO,Somalia,AF,.so,SOS,Shilling,"so-SO,ar-SO,it,en-SO"
SR,Suriname,SA,.sr,SRD,Dollar,"nl-SR,en,srn,hns,jv"
ST,Sao Tome and Principe,AF,.st,STD,Dobra,pt-ST
SV,El Salvador,NA,.sv,USD,Dollar,es-SV
SX,Sint Maarten,NA,.sx,ANG,Guilder,"nl,en"
SY,Syria,AS,.sy,SYP,Pound,"ar-SY,ku,hy,arc,fr,en"
SZ,Swaziland,AF,.sz,SZL,Lilangeni,"en-SZ,ss-SZ"
TC,Turks and Caicos Islands,NA,.tc,USD,Dollar,en-TC
TD,Chad,AF,.td,XAF,Franc,"fr-TD,ar-TD,sre"
TF,French Southern Territories,AN,.tf,EUR,Euro  ,fr
TG,Togo,AF,.tg,XOF,Franc,"fr-TG,ee,hna,kbp,dag,ha"
TH,Thailand,AS,.th,THB,Baht,"th,en"
TJ,Tajikistan,AS,.tj,TJS,Somoni,"tg,ru"
TK,Tokelau,OC,.tk,NZD,Dollar,"tkl,en-TK"
TL,East Timor,OC,.tl,USD,Dollar,"tet,pt-TL,id,en"
TM,Turkmenistan,AS,.tm,TMT,Manat,"tk,ru,uz"
TN,Tunisia,AF,.tn,TND,Dinar,"ar-TN,fr"
TO,Tonga,OC,.to,TOP,Pa'anga,"to,en-TO"
TR,Turkey,AS,.tr,TRY,Lira,"tr-TR,ku,diq,az,av"
TT,Trinidad and Tobago,NA,.tt,TTD,Dollar,"en-TT,hns,fr,es,zh"
TV,Tuvalu,OC,.tv,AUD,Dollar,"tvl,en,sm,gil"
TW,Taiwan,AS,.tw,TWD,Dollar,"zh-TW,zh,nan,hak"
TZ,Tanzania,AF,.tz,TZS,Shilling,"sw-TZ,en,ar"
UA,Ukraine,EU,.ua,UAH,Hryvnia,"uk,ru-UA,rom,pl,hu"
UG,Uganda,AF,.ug,UGX,Shilling,"en-UG,lg,sw,ar"
UM,United States Minor Outlying Islands,OC,.um,USD,Dollar ,en-UM
US,United States,NA,.us,USD,Dollar,"en-US,es-US,haw,fr"
UY,Uruguay,SA,.uy,UYU,Peso,es-UY
UZ,Uzbekistan,AS,.uz,UZS,Som,"uz,ru,tg"
VA,Vatican,EU,.va,EUR,Euro,"la,it,fr"
VC,Saint Vincent and the Grenadines,NA,.vc,XCD,Dollar,"en-VC,fr"
VE,Venezuela,SA,.ve,VEF,Bolivar,es-VE
VG,British Virgin Islands,NA,.vg,USD,Dollar,en-VG
VI,U.S. Virgin Islands,NA,.vi,USD,Dollar,en-VI
VN,Vietnam,AS,.vn,VND,Dong,"vi,en,fr,zh,km"
VU,Vanuatu,OC,.vu,VUV,Vatu,"bi,en-VU,fr-VU"
WF,Wallis and Futuna,OC,.wf,XPF,Franc,"wls,fud,fr-WF"
WS,Samoa,OC,.ws,WST,Tala,"sm,en-WS"
YE,Yemen,AS,.ye,YER,Rial,ar-YE
YT,Mayotte,AF,.yt,EUR,Euro,fr-YT
ZA,South Africa,AF,.za,ZAR,Rand,"en-ZA,zu,xh,af,nso,tn,st,ts,ss,ve,nr"
ZM,Zambia,AF,.zm,ZMW,Kwacha,"en-ZM,bem,loz,lun,lue,ny,toi"
ZW,Zimbabwe,AF,.zw,ZWL,Dollar,"en-ZW,sn,nr,nd"
CS,Serbia and Montenegro,EU,.cs,RSD,Dinar,"cu,hu,sq,sr"
AN,Netherlands Antilles,NA,.an,ANG,Guilder,"nl-AN,en,es"
`
