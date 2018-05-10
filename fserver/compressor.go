/**
 * @brief		File's Compressor Tools
 * @author		barry
 * @date		2018/4/10
 */
package fserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Package Initialization
func init() {
}

///////////////////////////////////// Data Record IO Wrapper ///////////////////////////////////////////
type CompressHandles struct {
	TarFile    *os.File     // .tar file handle
	GZipWriter *gzip.Writer // gzip.Writer handle
	TarWriter  *tar.Writer  // tar.Writer handle
}

func (pSelf *CompressHandles) OpenFile(sFilePath string, nGZipCompressLevel int) bool {
	var err error

	pSelf.TarFile, err = os.Create(sFilePath)
	if err != nil {
		log.Println("[ERR] CompressHandles.OpenFile() : failed 2 create *.tar file :", sFilePath, err.Error())
		return false
	}

	pSelf.GZipWriter, err = gzip.NewWriterLevel(pSelf.TarFile, nGZipCompressLevel)
	if err != nil {
		log.Println("[ERR] CompressHandles.OpenFile() : failed 2 create *tar.Writer :", sFilePath, err.Error())
		return false
	}

	pSelf.TarWriter = tar.NewWriter(pSelf.GZipWriter)
	if err != nil {
		log.Println("[ERR] CompressHandles.OpenFile() : failed 2 create *gzip.Writer :", sFilePath, err.Error())
		return false
	}

	log.Printf("[INF] CompressHandles.OpenFile() : [OK] (%s)", sFilePath)
	return true
}

func (pSelf *CompressHandles) CloseFile() {
	if pSelf.GZipWriter != nil {
		pSelf.GZipWriter.Close()
	}
	if pSelf.TarWriter != nil {
		pSelf.TarWriter.Close()
	}
	if pSelf.TarFile != nil {
		pSelf.TarFile.Close()
		log.Printf("[INF] CompressHandles.CloseFile() : [OK] (%s)", pSelf.TarFile.Name())
	}
}

type I_Record_IO interface {
	Initialize() bool
	Release() []ResDownload
	LoadFromFile(bytesData []byte) ([]byte, int, int)   // load data from file, return [] byte (return nil means end of file)
	CodeInWhiteTable(sFileName string) bool             // judge whether the file need 2 be loaded
	GenFilePath(sFileName string) string                // generate name  of file which in .tar
	GrapWriter(sFilePath string, nDate int) *tar.Writer // grap a .tar writer ptr
	GetCompressLevel() int                              // get gzip compression level
}

type BaseRecordIO struct {
	DataType        string
	CodeRangeFilter I_Range_OP
	mapFileHandle   map[string]CompressHandles
}

func (pSelf *BaseRecordIO) GetCompressLevel() int {
	return gzip.DefaultCompression
}

func (pSelf *BaseRecordIO) CodeInWhiteTable(sFileName string) bool {
	return true
}

func (pSelf *BaseRecordIO) Initialize() bool {
	pSelf.mapFileHandle = make(map[string]CompressHandles)
	return true
}

func (pSelf *BaseRecordIO) Release() []ResDownload {
	var byteMD5 []byte
	var lstRes []ResDownload
	var lstSortKeys []string
	log.Println("[INF] BaseRecordIO.Release() : flushing files 2 disk, count =", len(pSelf.mapFileHandle))

	for sPath, objHandles := range pSelf.mapFileHandle {
		objHandles.CloseFile()
		lstSortKeys = append(lstSortKeys, sPath)
	}

	sort.Strings(lstSortKeys)
	for _, sVal := range lstSortKeys {
		objMd5File, err := os.Open(sVal)
		if err != nil {
			log.Println("[WARN] BaseRecordIO.Release() : local file is not exist :", sVal)
			return lstRes
		}
		defer objMd5File.Close()
		/////////////////////// Generate MD5 String
		objMD5Hash := md5.New()
		if _, err := io.Copy(objMD5Hash, objMd5File); err != nil {
			log.Printf("[WARN] BaseRecordIO.Release() : failed 2 generate MD5 : %s : %s", sVal, err.Error())
			return lstRes
		}

		sMD5 := strings.ToLower(fmt.Sprintf("%x", objMD5Hash.Sum(byteMD5)))
		log.Printf("[INF] BaseRecordIO.Release() : close file = %s, md5 = %s", sVal, sMD5)
		lstRes = append(lstRes, ResDownload{TYPE: pSelf.DataType, URI: sVal, MD5: sMD5, UPDATE: time.Now().Format("2006-01-02 15:04:05")})
	}

	return lstRes
}

func (pSelf *BaseRecordIO) GenFilePath(sFileName string) string {
	return sFileName
}

func (pSelf *BaseRecordIO) GrapWriter(sFilePath string, nDate int) *tar.Writer {
	var sFile string = ""
	var objToday time.Time = time.Now()

	objRecordDate := time.Date(nDate/10000, time.Month(nDate%10000/100), nDate%100, 21, 6, 9, 0, time.Local)
	subHours := objToday.Sub(objRecordDate)
	nDays := subHours.Hours() / 24

	if nDays <= 16 { ////// Current Month
		sFile = fmt.Sprintf("%s%d", sFilePath, nDate)
	} else { ////////////////////////// Not Current Month
		nDD := (nDate % 100) ////////// One File With 2 Week's Data Inside
		if nDD <= 15 {
			nDD = 0
		} else {
			nDD = 15
		}
		sFile = fmt.Sprintf("%s%d", sFilePath, nDate/100*100+nDD)
	}

	if objHandles, ok := pSelf.mapFileHandle[sFile]; ok {
		return objHandles.TarWriter
	} else {
		var objCompressHandles CompressHandles

		if true == objCompressHandles.OpenFile(sFile, pSelf.GetCompressLevel()) {
			pSelf.mapFileHandle[sFile] = objCompressHandles

			return pSelf.mapFileHandle[sFile].TarWriter
		} else {
			log.Println("[ERR] BaseRecordIO.GrapWriter() : failed 2 open *tar.Writer :", sFilePath)
		}

	}

	return nil
}

///////////////////////////////////// Resources Compressor ///////////////////////////////////////////
type Compressor struct {
	TargetFolder string // Root Folder
}

///////////////////////////////////// Private Method ///////////////////////////////////////////
// Compress Folder Recursively
func (pSelf *Compressor) compressFolder(sDestFile string, sSrcFolder string, sRecursivePath string, pILoader I_Record_IO) bool {
	oDirFile, err := os.Open(sSrcFolder) // Open source diretory
	if err != nil {
		return false
	}
	defer oDirFile.Close()

	lstFileInfo, err := oDirFile.Readdir(0) // Get file info slice
	if err != nil {
		return false
	}

	for _, oFileInfo := range lstFileInfo {
		sCurPath := path.Join(sSrcFolder, oFileInfo.Name()) // Append path
		if oFileInfo.IsDir() {                              // Check it is directory or file
			pSelf.compressFolder(sDestFile, sCurPath, path.Join(sRecursivePath, oFileInfo.Name()), pILoader) // (Directory won't add unitl all subfiles are added)
		}

		compressFile(sDestFile, sCurPath, path.Join(sRecursivePath, oFileInfo.Name()), oFileInfo, pILoader)
	}

	return true
}

// Compress A File ( gzip + tar )
func compressFile(sDestFile string, sSrcFile string, sRecursivePath string, oFileInfo os.FileInfo, pILoader I_Record_IO) bool {
	if oFileInfo.IsDir() {
	} else {
		// Code File Is Not In White Table
		if pILoader.CodeInWhiteTable(sSrcFile) == false {
			return true
		}

		var nIndex int = 0
		oSrcFile, err := os.Open(sSrcFile) // File reader
		if err != nil {
			return false
		}
		defer oSrcFile.Close()
		bytesData, err := ioutil.ReadAll(oSrcFile)
		if err != nil {
			return false
		}

		for {
			hdr := new(tar.Header) // Create tar header
			//hdr, err := tar.FileInfoHeader(oFileInfo, "")
			hdr.Name = pILoader.GenFilePath(sRecursivePath)
			bData, nDate, nOffset := pILoader.LoadFromFile(bytesData[nIndex:])
			if len(bData) <= 0 {
				break
			}

			nIndex += nOffset
			pTarWriter := pILoader.GrapWriter(pILoader.GenFilePath(sDestFile), nDate)
			if nil == pTarWriter {
				return false
			}

			hdr.Size = int64(len(bData))
			hdr.Mode = int64(oFileInfo.Mode())
			hdr.ModTime = oFileInfo.ModTime()
			err = pTarWriter.WriteHeader(hdr) // Write hander
			if err != nil {
				return false
			}

			pTarWriter.Write(bData) // Write file data
		}
	}

	return true
}

///////////////////////////////////// [OutterMethod] ///////////////////////////////////////////
// [method] XCompress
func (pSelf *Compressor) XCompress(sResType string, objDataSrc *DataSourceConfig, codeRange I_Range_OP) ([]ResDownload, bool) {
	var lstRes []ResDownload
	var sDataType string = strings.ToLower(sResType[strings.Index(sResType, "."):])              // data type (d1/m1/m5/wt)
	var sDestFolder string = filepath.Join(pSelf.TargetFolder, strings.ToUpper(objDataSrc.MkID)) // target folder of data(.tar.gz)

	sDestFolder = strings.Replace(sDestFolder, "\\", "/", -1)
	log.Printf("[INF] Compressor.XCompress() : [Compressing] ExchangeCode:%s, DataType:%s, Folder:%s", objDataSrc.MkID, sDataType, objDataSrc.Folder)

	switch {
	case (objDataSrc.MkID == "sse" && sDataType == ".wt") || (objDataSrc.MkID == "szse" && sDataType == ".wt"):
		objRecordIO := WeightRecordIO{BaseRecordIO: BaseRecordIO{CodeRangeFilter: codeRange, DataType: strings.ToLower(sResType)}} // policy of Weight data loader
		return pSelf.translateFolder(filepath.Join(sDestFolder, "WEIGHT/WEIGHT."), objDataSrc.Folder, &objRecordIO)
	case (objDataSrc.MkID == "sse" && sDataType == ".d1") || (objDataSrc.MkID == "szse" && sDataType == ".d1"):
		objRecordIO := Day1RecordIO{BaseRecordIO: BaseRecordIO{CodeRangeFilter: codeRange, DataType: strings.ToLower(sResType)}} // policy of Day data loader
		return pSelf.translateFolder(filepath.Join(sDestFolder, "DAY/DAY."), objDataSrc.Folder, &objRecordIO)
	case (objDataSrc.MkID == "sse" && sDataType == ".m1") || (objDataSrc.MkID == "szse" && sDataType == ".m1"):
		objRecordIO := Minutes1RecordIO{BaseRecordIO: BaseRecordIO{CodeRangeFilter: codeRange, DataType: strings.ToLower(sResType)}} // policy of M1 data loader
		return pSelf.translateFolder(filepath.Join(sDestFolder, "MIN/MIN."), objDataSrc.Folder, &objRecordIO)
	case (objDataSrc.MkID == "sse" && sDataType == ".m5") || (objDataSrc.MkID == "szse" && sDataType == ".m5"):
		objRecordIO := Minutes5RecordIO{BaseRecordIO: BaseRecordIO{CodeRangeFilter: codeRange, DataType: strings.ToLower(sResType)}} // policy of M5 data loader
		return pSelf.translateFolder(filepath.Join(sDestFolder, "MIN5/MIN5."), objDataSrc.Folder, &objRecordIO)
	case (objDataSrc.MkID == "sse" && sDataType == ".m60") || (objDataSrc.MkID == "szse" && sDataType == ".m60"):
		objRecordIO := Minutes60RecordIO{BaseRecordIO: BaseRecordIO{CodeRangeFilter: codeRange, DataType: strings.ToLower(sResType)}} // policy of M60 data loader
		return pSelf.translateFolder(filepath.Join(sDestFolder, "MIN60/MIN60."), objDataSrc.Folder, &objRecordIO)
	default:
		log.Printf("[ERR] Compressor.XCompress() : [Compressing] invalid exchange code(%s) or data type(%s)", objDataSrc.MkID, sDataType)
		return lstRes, false
	}
}

///////////////////////////////////// [InnerMethod] ///////////////////////////////////////////
// [Method] load source data 2 targer folder
func (pSelf *Compressor) translateFolder(sDestFile, sSrcFolder string, pILoader I_Record_IO) ([]ResDownload, bool) {
	var lstRes []ResDownload
	var sMkFolder string = path.Dir(sDestFile)
	//////////////// Prepare Data Folder && File Handles
	if "windows" == runtime.GOOS {
		sMkFolder = sDestFile[:strings.LastIndex(sDestFile, pathSep)]
	}
	sDestFile = strings.Replace(sDestFile, "\\", "/", -1)
	err := os.MkdirAll(sMkFolder, 0755)
	if err != nil {
		log.Println("[ERR] Compressor.translateFolder() : cannot build target folder 4 zip file :", sMkFolder)
		return lstRes, false
	}
	///////////////// Initialize Object type(I_Record_IO)
	log.Printf("[INF] Compressor.translateFolder() : compressing ---> (%s)", sSrcFolder)
	if false == pILoader.Initialize() {
		log.Println("[ERR] Compressor.translateFolder() : Cannot initialize I_Record_IO object, ", sSrcFolder)
		return lstRes, false
	}
	///////////////// Compressing Source Data Folder
	sDestFile = pILoader.GenFilePath(sDestFile)
	if "windows" != runtime.GOOS {
		if false == pSelf.compressFolder(sDestFile, sSrcFolder, path.Base(sSrcFolder), pILoader) {
			return lstRes, false
		}
	} else {
		if false == pSelf.compressFolder(sDestFile, sSrcFolder, "./", pILoader) {
			return lstRes, false
		}
	}

	return pILoader.Release(), true
}

///////////////////////// 60Minutes Lines ///////////////////////////////////////////
type Minutes60RecordIO struct {
	BaseRecordIO
}

func (pSelf *Minutes60RecordIO) CodeInWhiteTable(sFileName string) bool {
	if pSelf.CodeRangeFilter == nil {
		return true
	}

	nEnd := strings.LastIndexAny(sFileName, ".")
	nFileYear, err := strconv.Atoi(sFileName[nEnd-4 : nEnd])
	if nil != err {
		log.Println("[ERR] Minutes1RecordIO.CodeInWhiteTable() : Year In FileName is not digital: ", sFileName, nFileYear)
		return false
	}
	if time.Now().Year()-nFileYear >= 2 {
		return false
	}
	nBegin := strings.LastIndexAny(sFileName, "MIN")
	nEnd = nEnd - 5
	sCodeNum := sFileName[nBegin+1 : nEnd]

	return pSelf.CodeRangeFilter.CodeInRange(sCodeNum)
}

func (pSelf *Minutes60RecordIO) GenFilePath(sFileName string) string {
	return strings.Replace(sFileName, "MIN/", "MIN60/", -1)
}

func (pSelf *Minutes60RecordIO) LoadFromFile(bytesData []byte) ([]byte, int, int) {
	var err error
	var nOffset int = 0
	var bLine []byte
	var i int = 0
	var nReturnDate int = -100
	var objToday time.Time = time.Now()
	var rstr string = ""
	var objMin60 struct {
		Date         int     // date
		Time         int     // time
		Open         float64 // open price
		High         float64 // high price
		Low          float64 // low price
		Close        float64 // close price
		Settle       float64 // settle price
		Amount       float64 // Amount
		Volume       int64   // Volume
		OpenInterest int64   // Open Interest
		NumTrades    int64   // Trade Number
		Voip         float64 // Voip
	} // 60 minutes k-line

	bLines := bytes.Split(bytesData, []byte("\n"))
	nCount := len(bLines)
	for i, bLine = range bLines {
		nOffset += (len(bLine) + 1)
		lstRecords := strings.Split(string(bLine), ",")
		if len(lstRecords[0]) <= 0 {
			continue
		}
		objMin60.Date, err = strconv.Atoi(lstRecords[0])
		if err != nil {
			continue
		}

		objRecordDate := time.Date(objMin60.Date/10000, time.Month(objMin60.Date%10000/100), objMin60.Date%100, 21, 6, 9, 0, time.Local)
		subHours := objToday.Sub(objRecordDate)
		nDays := subHours.Hours() / 24
		if nDays > 366 {
			continue
		}

		if -100 == nReturnDate {
			nReturnDate = objMin60.Date
		}

		if nReturnDate != objMin60.Date {
			return []byte(rstr), nReturnDate, nOffset
		}

		// cal. 60 minutes k-lines
		nCurTime, _ := strconv.Atoi(lstRecords[1])
		nCurTime /= 10000000
		objMin60.Close, _ = strconv.ParseFloat(lstRecords[5], 64)
		objMin60.Settle, _ = strconv.ParseFloat(lstRecords[6], 64)
		objMin60.Voip, _ = strconv.ParseFloat(lstRecords[11], 64)

		if objMin60.Time == 0 {
			objMin60.Time = (nCurTime + 1) * 10000
			objMin60.Open, _ = strconv.ParseFloat(lstRecords[2], 64)
			objMin60.High, _ = strconv.ParseFloat(lstRecords[3], 64)
			objMin60.Low, _ = strconv.ParseFloat(lstRecords[4], 64)
			objMin60.Amount, _ = strconv.ParseFloat(lstRecords[7], 64)
			objMin60.Volume, _ = strconv.ParseInt(lstRecords[8], 10, 64)
			objMin60.OpenInterest, _ = strconv.ParseInt(lstRecords[9], 10, 64)
			objMin60.NumTrades, _ = strconv.ParseInt(lstRecords[10], 10, 64)
			rstr += fmt.Sprintf("%d,%d,%f,%f,%f,%f,%f,%f,%d,%d,%d,%f\n", objMin60.Date, objMin60.Time, objMin60.Open, objMin60.High, objMin60.Low, objMin60.Close, objMin60.Settle, objMin60.Amount, objMin60.Volume, objMin60.OpenInterest, objMin60.NumTrades, objMin60.Voip)
		}

		if objMin60.Time <= nCurTime*10000 { // begin
			//if 0 != i {
			rstr += fmt.Sprintf("%d,%d,%f,%f,%f,%f,%f,%f,%d,%d,%d,%f\n", objMin60.Date, objMin60.Time, objMin60.Open, objMin60.High, objMin60.Low, objMin60.Close, objMin60.Settle, objMin60.Amount, objMin60.Volume, objMin60.OpenInterest, objMin60.NumTrades, objMin60.Voip)
			//}

			objMin60.Time = (nCurTime + 1) * 10000
			objMin60.Open, _ = strconv.ParseFloat(lstRecords[2], 64)
			objMin60.High, _ = strconv.ParseFloat(lstRecords[3], 64)
			objMin60.Low, _ = strconv.ParseFloat(lstRecords[4], 64)
			objMin60.Amount, _ = strconv.ParseFloat(lstRecords[7], 64)
			objMin60.Volume, _ = strconv.ParseInt(lstRecords[8], 10, 64)
			objMin60.OpenInterest, _ = strconv.ParseInt(lstRecords[9], 10, 64)
			objMin60.NumTrades, _ = strconv.ParseInt(lstRecords[10], 10, 64)
		} else {
			nHigh, _ := strconv.ParseFloat(lstRecords[3], 64)
			nLow, _ := strconv.ParseFloat(lstRecords[4], 64)
			if nHigh > objMin60.High {
				objMin60.High = nHigh
			}
			if nLow > objMin60.Low {
				objMin60.Low = nLow
			}
			nAmount, _ := strconv.ParseFloat(lstRecords[7], 64)
			objMin60.Amount += nAmount
			nVolume, _ := strconv.ParseInt(lstRecords[8], 10, 64)
			objMin60.Volume += nVolume
			nOpenInterest, _ := strconv.ParseInt(lstRecords[9], 10, 64)
			objMin60.OpenInterest += nOpenInterest
			nNumTrades, _ := strconv.ParseInt(lstRecords[10], 10, 64)
			objMin60.NumTrades += nNumTrades
		}

		if i == (nCount - 1) {
			rstr += fmt.Sprintf("%d,%d,%f,%f,%f,%f,%f,%f,%d,%d,%d,%f\n", objMin60.Date, objMin60.Time, objMin60.Open, objMin60.High, objMin60.Low, objMin60.Close, objMin60.Settle, objMin60.Amount, objMin60.Volume, objMin60.OpenInterest, objMin60.NumTrades, objMin60.Voip)
		}
	}

	return []byte(rstr), nReturnDate, len(bytesData)
}

///////////////////////// 5Minutes Lines ///////////////////////////////////////////
type Minutes5RecordIO struct {
	BaseRecordIO
}

func (pSelf *Minutes5RecordIO) CodeInWhiteTable(sFileName string) bool {
	if pSelf.CodeRangeFilter == nil {
		return true
	}

	nEnd := strings.LastIndexAny(sFileName, ".")
	nFileYear, err := strconv.Atoi(sFileName[nEnd-4 : nEnd])
	if nil != err {
		log.Println("[ERR] Minutes1RecordIO.CodeInWhiteTable() : Year In FileName is not digital: ", sFileName, nFileYear)
		return false
	}
	if time.Now().Year()-nFileYear >= 2 {
		return false
	}
	nBegin := strings.LastIndexAny(sFileName, "MIN")
	nEnd = nEnd - 5
	sCodeNum := sFileName[nBegin+1 : nEnd]

	return pSelf.CodeRangeFilter.CodeInRange(sCodeNum)
}

func (pSelf *Minutes5RecordIO) GenFilePath(sFileName string) string {
	return strings.Replace(sFileName, "MIN/", "MIN5/", -1)
}

func (pSelf *Minutes5RecordIO) LoadFromFile(bytesData []byte) ([]byte, int, int) {
	var err error
	var nOffset int = 0
	var bLine []byte
	var i int = 0
	var nReturnDate int = -100
	var objToday time.Time = time.Now()
	var rstr string = ""
	var objMin5 struct {
		Date         int     // date
		Time         int     // time
		Open         float64 // open price
		High         float64 // high price
		Low          float64 // low price
		Close        float64 // close price
		Settle       float64 // settle price
		Amount       float64 // Amount
		Volume       int64   // Volume
		OpenInterest int64   // Open Interest
		NumTrades    int64   // Trade Number
		Voip         float64 // Voip
	} // 5 minutes k-line

	bLines := bytes.Split(bytesData, []byte("\n"))
	nCount := len(bLines)
	for i, bLine = range bLines {
		nOffset += (len(bLine) + 1)
		lstRecords := strings.Split(string(bLine), ",")
		if len(lstRecords[0]) <= 0 {
			continue
		}
		objMin5.Date, err = strconv.Atoi(lstRecords[0])
		if err != nil {
			continue
		}

		objRecordDate := time.Date(objMin5.Date/10000, time.Month(objMin5.Date%10000/100), objMin5.Date%100, 21, 6, 9, 0, time.Local)
		subHours := objToday.Sub(objRecordDate)
		nDays := subHours.Hours() / 24
		if nDays > 366 {
			continue
		}

		if -100 == nReturnDate {
			nReturnDate = objMin5.Date
		}

		if nReturnDate != objMin5.Date {
			return []byte(rstr), nReturnDate, nOffset
		}

		// cal. 5 minutes k-lines
		nCurTime, _ := strconv.Atoi(lstRecords[1])
		nCurTime /= 100000
		objMin5.Close, _ = strconv.ParseFloat(lstRecords[5], 64)
		objMin5.Settle, _ = strconv.ParseFloat(lstRecords[6], 64)
		objMin5.Voip, _ = strconv.ParseFloat(lstRecords[11], 64)

		if objMin5.Time == 0 {
			objMin5.Time = (nCurTime + 5) * 100
			objMin5.Open, _ = strconv.ParseFloat(lstRecords[2], 64)
			objMin5.High, _ = strconv.ParseFloat(lstRecords[3], 64)
			objMin5.Low, _ = strconv.ParseFloat(lstRecords[4], 64)
			objMin5.Amount, _ = strconv.ParseFloat(lstRecords[7], 64)
			objMin5.Volume, _ = strconv.ParseInt(lstRecords[8], 10, 64)
			objMin5.OpenInterest, _ = strconv.ParseInt(lstRecords[9], 10, 64)
			objMin5.NumTrades, _ = strconv.ParseInt(lstRecords[10], 10, 64)
			rstr += fmt.Sprintf("%d,%d,%f,%f,%f,%f,%f,%f,%d,%d,%d,%f\n", objMin5.Date, objMin5.Time, objMin5.Open, objMin5.High, objMin5.Low, objMin5.Close, objMin5.Settle, objMin5.Amount, objMin5.Volume, objMin5.OpenInterest, objMin5.NumTrades, objMin5.Voip)
		}

		if objMin5.Time <= nCurTime*100 { // begin
			//if 0 != i {
			rstr += fmt.Sprintf("%d,%d,%f,%f,%f,%f,%f,%f,%d,%d,%d,%f\n", objMin5.Date, objMin5.Time, objMin5.Open, objMin5.High, objMin5.Low, objMin5.Close, objMin5.Settle, objMin5.Amount, objMin5.Volume, objMin5.OpenInterest, objMin5.NumTrades, objMin5.Voip)
			//}

			objMin5.Time = (nCurTime + 5) * 100
			objMin5.Open, _ = strconv.ParseFloat(lstRecords[2], 64)
			objMin5.High, _ = strconv.ParseFloat(lstRecords[3], 64)
			objMin5.Low, _ = strconv.ParseFloat(lstRecords[4], 64)
			objMin5.Amount, _ = strconv.ParseFloat(lstRecords[7], 64)
			objMin5.Volume, _ = strconv.ParseInt(lstRecords[8], 10, 64)
			objMin5.OpenInterest, _ = strconv.ParseInt(lstRecords[9], 10, 64)
			objMin5.NumTrades, _ = strconv.ParseInt(lstRecords[10], 10, 64)
		} else {
			nHigh, _ := strconv.ParseFloat(lstRecords[3], 64)
			nLow, _ := strconv.ParseFloat(lstRecords[4], 64)
			if nHigh > objMin5.High {
				objMin5.High = nHigh
			}
			if nLow > objMin5.Low {
				objMin5.Low = nLow
			}
			nAmount, _ := strconv.ParseFloat(lstRecords[7], 64)
			objMin5.Amount += nAmount
			nVolume, _ := strconv.ParseInt(lstRecords[8], 10, 64)
			objMin5.Volume += nVolume
			nOpenInterest, _ := strconv.ParseInt(lstRecords[9], 10, 64)
			objMin5.OpenInterest += nOpenInterest
			nNumTrades, _ := strconv.ParseInt(lstRecords[10], 10, 64)
			objMin5.NumTrades += nNumTrades
		}

		if i == (nCount - 1) {
			rstr += fmt.Sprintf("%d,%d,%f,%f,%f,%f,%f,%f,%d,%d,%d,%f\n", objMin5.Date, objMin5.Time, objMin5.Open, objMin5.High, objMin5.Low, objMin5.Close, objMin5.Settle, objMin5.Amount, objMin5.Volume, objMin5.OpenInterest, objMin5.NumTrades, objMin5.Voip)
		}
	}

	return []byte(rstr), nReturnDate, len(bytesData)
}

///////////////////////// 1Minutes Lines ///////////////////////////////////////////
type Minutes1RecordIO struct {
	BaseRecordIO
}

func (pSelf *Minutes1RecordIO) CodeInWhiteTable(sFileName string) bool {
	if pSelf.CodeRangeFilter == nil {
		return true
	}

	nEnd := strings.LastIndexAny(sFileName, ".")
	nFileYear, err := strconv.Atoi(sFileName[nEnd-4 : nEnd])
	if nil != err {
		log.Println("[ERR] Minutes1RecordIO.CodeInWhiteTable() : Year In FileName is not digital: ", sFileName, nFileYear)
		return false
	}
	if time.Now().Year()-nFileYear >= 2 {
		return false
	}
	nBegin := strings.LastIndexAny(sFileName, "MIN")
	nEnd = nEnd - 5
	sCodeNum := sFileName[nBegin+1 : nEnd]

	return pSelf.CodeRangeFilter.CodeInRange(sCodeNum)
}

func (pSelf *Minutes1RecordIO) LoadFromFile(bytesData []byte) ([]byte, int, int) {
	var nReturnDate int = -100
	var rstr string = ""
	var nOffset int = 0
	var objToday time.Time = time.Now()

	for _, bLine := range bytes.Split(bytesData, []byte("\n")) {
		nOffset += (len(bLine) + 1)
		sFirstFields := strings.Split(string(bLine), ",")[0]
		if len(sFirstFields) <= 0 {
			continue
		}
		nDate, err := strconv.Atoi(sFirstFields)
		if err != nil {
			continue
		}

		objRecordDate := time.Date(nDate/10000, time.Month(nDate%10000/100), nDate%100, 21, 6, 9, 0, time.Local)
		subHours := objToday.Sub(objRecordDate)
		nDays := subHours.Hours() / 24
		if nDays > 14 {
			continue
		}

		if -100 == nReturnDate {
			nReturnDate = nDate
		}

		if nReturnDate != nDate {
			return []byte(rstr), nReturnDate, nOffset
		}

		rstr += (string(bLine) + "\n")
	}

	return []byte(rstr), nReturnDate, len(bytesData)
}

///////////////////////// 1 Day Lines ///////////////////////////////////////////
type Day1RecordIO struct {
	BaseRecordIO
}

func (pSelf *Day1RecordIO) GetCompressLevel() int {
	return gzip.BestSpeed
}

func (pSelf *Day1RecordIO) GrapWriter(sFilePath string, nDate int) *tar.Writer {
	var sFile string = ""
	var objToday time.Time = time.Now()

	objRecordDate := time.Date(nDate/10000, time.Month(nDate%10000/100), nDate%100, 21, 6, 9, 0, time.Local)
	subHours := objToday.Sub(objRecordDate)
	nDays := subHours.Hours() / 24

	if nDays <= 16 { ////// Current Month
		sFile = fmt.Sprintf("%s%d", sFilePath, nDate)
	} else { ////////////////////////// Not Current Month
		sFile = fmt.Sprintf("%s%d", sFilePath, nDate/10000*10000)
	}

	if objHandles, ok := pSelf.mapFileHandle[sFile]; ok {
		return objHandles.TarWriter
	} else {
		var objCompressHandles CompressHandles

		if true == objCompressHandles.OpenFile(sFile, pSelf.GetCompressLevel()) {
			pSelf.mapFileHandle[sFile] = objCompressHandles

			return pSelf.mapFileHandle[sFile].TarWriter
		} else {
			log.Println("[ERR] Day1RecordIO.GrapWriter() : failed 2 open *tar.Writer :", sFilePath)
		}

	}

	return nil
}

func (pSelf *Day1RecordIO) CodeInWhiteTable(sFileName string) bool {
	if pSelf.CodeRangeFilter == nil {
		return true
	}

	nBegin := strings.LastIndexAny(sFileName, "DAY")
	nEnd := strings.LastIndexAny(sFileName, ".")
	sCodeNum := sFileName[nBegin+1 : nEnd]

	return pSelf.CodeRangeFilter.CodeInRange(sCodeNum)
}

func (pSelf *Day1RecordIO) LoadFromFile(bytesData []byte) ([]byte, int, int) {
	var nReturnDate int = -100
	var rstr string = ""
	var nOffset int = 0

	for _, bLine := range bytes.Split(bytesData, []byte("\n")) {
		nOffset += (len(bLine) + 1)
		sFirstFields := strings.Split(string(bLine), ",")[0]
		if len(sFirstFields) <= 0 {
			continue
		}
		nDate, err := strconv.Atoi(sFirstFields)
		if err != nil {
			continue
		}

		if -100 == nReturnDate {
			nReturnDate = nDate
		}

		if nReturnDate != nDate {
			return []byte(rstr), nReturnDate, nOffset
		}

		rstr += (string(bLine) + "\n")
	}

	return []byte(rstr), nReturnDate, len(bytesData)
}

///////////////////////// Weights Lines ///////////////////////////////////////////
type WeightRecordIO struct {
	BaseRecordIO
}

func (pSelf *WeightRecordIO) LoadFromFile(bytesData []byte) ([]byte, int, int) {
	return bytesData, 0, len(bytesData)
}
