/**
 * @brief		Engine Of Server
 * @author		barry
 * @date		2018/4/10
 */
package fserver

import (
	//"archive/zip"
	"encoding/xml"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"time"
)

func init() {
}

type DataSourceConfig struct {
	MkID       string    // market id ( SSE:shanghai SZSE:shenzheng )
	Folder     string    // data file folder
	MD5        string    // md5 of file
	UpdateTime time.Time // updatetime of file
}

///////////////////////////////////// File Scheduler Stucture/Class
type FileScheduler struct {
	XmlCfgPath       string                      // Xml Configuration File Path
	SyncFolder       string                      // Sync File Folder
	DataSourceConfig map[string]DataSourceConfig // Data Source Config Of Markets
	BuildTime        int                         // Resources' Build Time
	LastUpdateTime   time.Time                   // Last Updatetime
}

///////////////////////////////////// [OutterMethod]
//  Active File Scheduler
func (pSelf *FileScheduler) Active() bool {
	log.Println("[INF] FileScheduler.Active() : configuration file path: ", pSelf.XmlCfgPath)

	// Definition Of Profile's Structure
	var objCfg struct {
		XMLName xml.Name `xml:"cfg"`
		Version string   `xml:"version,attr"`
		Setting []struct {
			XMLName xml.Name `xml:"setting"`
			Name    string   `xml:"name,attr"`
			Value   string   `xml:"value,attr"`
		} `xml:"setting"`
	}

	// Analyze configuration(.xml) 4 Engine
	sXmlContent, err := ioutil.ReadFile(pSelf.XmlCfgPath)
	if err != nil {
		log.Println("[WARN] FileScheduler.Active() : cannot locate configuration file, path: ", pSelf.XmlCfgPath)
		return false
	}

	err = xml.Unmarshal(sXmlContent, &objCfg)
	if err != nil {
		log.Println("[WARN] FileScheduler.Active() : cannot parse xml configuration file, error: ", err.Error())
		return false
	}

	// Extract Settings
	log.Println("[INF] FileScheduler.Active() : [Xml.Setting] configuration file version: ", objCfg.Version)
	pSelf.LastUpdateTime = time.Now().AddDate(-1, 0, -1)
	pSelf.DataSourceConfig = make(map[string]DataSourceConfig)
	for _, objSetting := range objCfg.Setting {
		switch strings.ToLower(objSetting.Name) {
		case "buildtime":
			pSelf.BuildTime, _ = strconv.Atoi(objSetting.Value)
			log.Println("[INF] FileScheduler.Active() : [Xml.Setting] Build Time: ", pSelf.BuildTime)
		case "syncfolder":
			pSelf.SyncFolder = objSetting.Value
			log.Println("[INF] FileScheduler.Active() : [Xml.Setting] SyncFolder: ", pSelf.SyncFolder)
		default:
			sSetting := strings.ToLower(objSetting.Name)
			if len(strings.Split(objSetting.Name, ".")) <= 1 {
				log.Println("[WARN] FileScheduler.Active() : [Xml.Setting] Ignore -> ", objSetting.Name)
				continue
			}

			pSelf.DataSourceConfig[sSetting] = DataSourceConfig{MkID: strings.Split(objSetting.Name, ".")[0], Folder: objSetting.Value}
			log.Println("[INF] FileScheduler.Active() : [Xml.Setting] ", sSetting, pSelf.DataSourceConfig[sSetting].MkID, pSelf.DataSourceConfig[sSetting].Folder)
		}
	}

	return pSelf.BuildSyncResource()
}

func (pSelf *FileScheduler) BuildSyncResource() bool {
	objNowTime := time.Now()
	objBuildTime := time.Date(objNowTime.Year(), objNowTime.Month(), objNowTime.Day(), pSelf.BuildTime/10000, pSelf.BuildTime/100%100, pSelf.BuildTime%100, 0, time.Local)

	// need 2 initialize resouces
	if pSelf.LastUpdateTime.Year() != objBuildTime.Year() || pSelf.LastUpdateTime.Month() != objBuildTime.Month() || pSelf.LastUpdateTime.Day() != objBuildTime.Day() {
		if objNowTime.After(objBuildTime) == true {
			log.Printf("[INF] FileScheduler.BuildSyncResource() : (BuildTime=%s) Building sync resources... ", objBuildTime.Format("2006-01-02 15:04:05"))
			pSelf.LastUpdateTime = time.Now() // update time

			for key, value := range pSelf.DataSourceConfig {
				log.Printf("%s, %s", key, value.Folder)
			}

			log.Println("[INF] FileScheduler.BuildSyncResource() : Builded!")
		}
	}

	return true
}
