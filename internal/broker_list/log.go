package brokerlist

type Logger interface {
	Printf(fmt string, v ...interface{})
	Debugf(fmt string, v ...interface{})
	Infof(fmt string, v ...interface{})
	Warnf(fmt string, v ...interface{})
	Errorf(fmt string, v ...interface{})
}
